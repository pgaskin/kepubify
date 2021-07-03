import { KepubifyWasmInit, KepubifyWasmMessage, KepubifyWasmOption, KepubifyWasmURL } from "./wasm"
import KepubifyWorkerImpl from "web-worker:./worker"

type QueueTask = {
    epub: Blob,
    config: KepubifyWasmOption[],
    progress?: (n: number, total: number) => void,
    resolve: (kepub: Blob) => void,
    reject: (err: Error) => void,
}

export class Kepubify {
    #wasm?: WebAssembly.Module
    #activeWorkers = new Set() // to work around bugs with workers being GC'd early

    constructor() {
        // set some sane defaults for the queue
        if ((window as any).chrome) {
            // chrome has better memory management and performance for workers
            this.#queueLimitWorkers = Math.max(Math.min(navigator.hardwareConcurrency || 2, 1), /Mobi/i.test(window.navigator.userAgent) ? 2 : 8)
            this.#queueLimitFileSize = Math.max(Math.min(((navigator as any).deviceMemory || 2 as number)*1_000_000_000 / 2, 4_000_000_000) / this.#queueLimitWorkers, 50_000_000) / 2
            this.#queueLimitFileSizeHard = 300_000_000
        } else {
            this.#queueLimitWorkers = 2
            this.#queueLimitFileSize = 50_000_000
            this.#queueLimitFileSizeHard = 250_000_000
        }
        console.log(`[kepubify] automatically configured worker limits: workers=${this.#queueLimitWorkers} fileSize=${formatBytes(this.#queueLimitFileSize)} fileSizeHard=${formatBytes(this.#queueLimitFileSizeHard)}`)
    }

    async init(progress?: (lengthComputable: boolean, loaded: number, total: number) => void): Promise<string> {
        if (this.#wasm) {
            throw new Error("kepubify already loaded")
        }
        const buf = await get(KepubifyWasmURL, progress)
        const wasm = await WebAssembly.compile(buf)
        const version = await new KepubifyWorker(wasm).version
        this.#wasm = wasm
        return version
    }

    async kepubify(epub: Blob, config?: KepubifyConfig, progress?: (n: number, total: number) => void): Promise<Blob> {
        if (!this.#wasm) {
            throw new Error("kepubify not loaded")
        }
        return new Promise((resolve, reject) => {
            this.#queue.push({
                epub,
                config: config?.options ?? [],
                progress,
                resolve, reject,
            })
            this.#queueTryWork()
        })
    }

    #queue: QueueTask[] = []
    #queueCurrentFileSize: number = 0
    #queueCurrentWorkers: number = 0

    #queueStrictOrder: boolean = false
    #queueLimitFileSize: number
    #queueLimitFileSizeHard: number
    #queueLimitWorkers: number

    /** The maximum total file size which can be processed at a time. */
    get queueLimitFileSize(): number {
        return this.#queueLimitFileSize
    }
    set queueLimitFileSize(v: number) {
        this.#queueLimitFileSize = v
        this.#queueTryWork()
    }

    /** The maximum total file size which can be processed. */
    get queueLimitFileSizeHard(): number {
        return this.#queueLimitFileSizeHard
    }
    set queueMaximumFileSizeHard(v: number) {
        this.#queueLimitFileSizeHard = v
        this.#queueTryWork()
    }

    /** The maximum number of items which can be processed at once. */
    get queueLimitWorkers(): number {
        return this.#queueLimitWorkers
    }
    set queueLimitWorkers(v: number) {
        this.#queueLimitWorkers = v
        this.#queueTryWork()
    }

    /** Whether to try other queue items if the first would exceed the limit. */
    get queueStrictOrder(): boolean {
        return this.#queueStrictOrder
    }
    set queueStrictOrder(v: boolean) {
        this.#queueStrictOrder = v
        this.#queueTryWork()
    }

    #queueTryWork() {
        // note: this is not racy since JS is single-threaded, and we don't have
        // any async/awaits in here
        while (this.#queue.length) {
            if (this.#queueLimitWorkers && this.#queueCurrentWorkers >= this.#queueLimitWorkers) {
                break
            }
            let firstMatch = 0
            for (let i = 0; i < this.#queue.length; i++) {
                // filter out items exceeding the hard limit
                if (this.#queueLimitFileSizeHard && this.#queue[i].epub.size > this.#queueLimitFileSizeHard) {
                    this.#queue[i].reject(new Error(`File too large for web version (maximum: ${formatBytes(this.#queueLimitFileSizeHard)})`))
                    this.#queue.splice(i--, 1)
                    continue
                }
                // filter out files exceeding the soft limit if there are active workers
                if (this.#queueLimitFileSize && this.#queueCurrentFileSize) {
                    if (this.#queueCurrentFileSize + this.#queue[i].epub.size > this.#queueLimitFileSize) {
                        if (this.#queueStrictOrder) {
                            firstMatch = -2
                            break
                        } else {
                            continue
                        }
                    }
                }
                // if it wasn't filtered out, use it
                firstMatch = i
                break
            }
            if (firstMatch == -2) {
                break
            }
            this.#queueDoWork(firstMatch)
        }
    }

    #queueDoWork(i: number): Promise<void> {
        const { epub, config, progress, resolve, reject } = this.#queue[i]
        this.#queue.splice(i, 1)

        this.#queueCurrentWorkers++
        this.#queueCurrentFileSize += epub.size

        return (async () => {
            let worker
            try {
                const buf = await readAsArrayBuffer(epub)
                this.#activeWorkers.add(worker = new KepubifyWorker(this.#wasm!, {
                    epub: buf,
                    config,
                    progress: !!progress,
                }, progress))
                const kepub = await worker.kepub
                resolve(new Blob([kepub], { type: "application/epub+zip" }))
            } catch (ex) {
                reject(ex)
            } finally {
                if (worker) {
                    this.#activeWorkers.delete(worker)
                }
            }

            this.#queueCurrentFileSize -= epub.size
            this.#queueCurrentWorkers--
            this.#queueTryWork()
        })()
    }
}

/** @internal */
export class KepubifyWorker {
    #worker?: Worker

    #kepub: Promise<ArrayBuffer>
    #resolve?: (kepub: ArrayBuffer) => void
    #reject?: (err: Error) => void

    #version: Promise<string>
    #resolveV?: (version: string) => void
    #rejectV?: (err: Error) => void

    #progress?: (n: number, total: number) => void

    #timeout?: any

    constructor(wasm: WebAssembly.Module, init?: KepubifyWasmInit, progress?: (n: number, total: number) => void) {
        this.#kepub = new Promise((resolve, reject) => {
            this.#resolve = x => {
                resolve(x)
                this.#resolve = undefined
            }
            this.#reject = x => {
                reject(x)
                this.#reject = undefined
            }
        })

        // exit after resolve/reject
        this.#kepub.then(
            () => {
                if (this.#worker) {
                    this.#worker.onerror = null
                    this.#worker.onmessage = null
                }
                window.setTimeout(() => this.terminate(), 100)
            },
            () => {
                if (this.#worker) {
                    this.#worker.onerror = null
                    this.#worker.onmessage = null
                }
                window.setTimeout(() => this.terminate(), 100)
            },
        )

        // version promise
        this.#version = new Promise((resolve, reject) => {
            this.#resolveV = x => {
                resolve(x)
                this.#resolveV = undefined
            }
            this.#rejectV = x => {
                reject(x)
                this.#rejectV = undefined
            }
        })

        // initialization timeout (for returning the version)
        this.#timeout = setTimeout(() => {
            const err = new Error("worker did not start in time")
            this.#rejectV?.(err)
            this.#reject?.(err)
        }, 2000)

        // conversion progress
        this.#progress = progress

        // start the worker
        this.#worker = new KepubifyWorkerImpl()
        this.#worker!.onmessage = this.#message.bind(this)
        this.#worker!.onerror = this.#error.bind(this)
        this.#worker!.postMessage({ wasm, init: init })
    }

    terminate() {
        const err = new Error("worker terminated")
        this.#rejectV?.(err)
        this.#reject?.(err)
        this.#worker?.terminate()
        this.#worker = undefined
    }

    /** Gets the promise which resolves with the version after worker init. */
    get version(): Promise<string> {
        return this.#version
    }

    /** Gets the promise which resolves with the kepub. */
    get kepub(): Promise<ArrayBuffer> {
        return this.#kepub
    }

    /** Handles uncaught errors, which should only occur during init. */
    #error(ev: ErrorEvent) {
        const err = ev.error instanceof Error
            ? ev.error
            : new Error(`${ev.message} (at ${ev.filename}:${ev.lineno}:${ev.colno})`)
        this.#rejectV?.(err)
        this.#reject?.(err)
    }

    /** Handles messages from the Go module. */
    #message(ev: MessageEvent) {
        let err
        const data = ev.data as KepubifyWasmMessage
        switch (data.type) {
            /** initialization */
            case "version":
                clearTimeout(this.#timeout)
                this.#resolveV?.(data.version)
                break

            /** conversion */
            case "progress":
                this.#progress?.(data.n, data.total)
                break
            case "success":
                this.#resolve?.(data.kepub)
                break
            case "error":
                err = new KepubifyError(data.message)
                this.#reject?.(err)
                break

            /** exit */
            case "panic":
                err = new Error(`worker panicked: ${data.message}`)
                err.stack = data.message + "\n\n" + data.stack
                this.#rejectV?.(err)
                this.#reject?.(err)
                break
            case "exit":
                err = new Error("worker exited prematurely")
                this.#rejectV?.(err)
                this.#reject?.(err)
                break
        }
    }
}

export class KepubifyConfig {
    #options: KepubifyWasmOption[] = []

    /** @internal */
    get options(): KepubifyWasmOption[] {
        return this.#options
    }

    smartypants(): this {
        this.#options.push({ type: "Smartypants" })
        return this
    }

    findReplace(find: string, replace: string): this {
        this.#options.push({ type: "FindReplace", find, replace })
        return this
    }

    dummyTitlepage(add: boolean): this {
        this.#options.push({ type: "DummyTitlepage", add })
        return this
    }

    addCSS(css: string): this {
        this.#options.push({ type: "AddCSS", css })
        return this
    }

    hyphenate(hyphenate: boolean): this {
        this.#options.push({ type: "Hyphenate", hyphenate })
        return this
    }

    fullScreenFixes(): this {
        this.#options.push({ type: "FullScreenFixes" })
        return this
    }
}

export class KepubifyError extends Error {
    constructor(message?: string) {
        super(message)
        this.name = KepubifyError.name
    }
}

async function readAsArrayBuffer(blob: Blob, progress?: (lengthComputable: boolean, loaded: number, total: number) => void): Promise<ArrayBuffer> {
    return new Promise((resolve, reject) => {
        const rd = new FileReader()
        rd.addEventListener("abort", ev => {
            progress?.(ev.lengthComputable, ev.loaded, ev.total)
            reject(new Error("aborted"))
        })
        rd.addEventListener("error", ev => {
            progress?.(ev.lengthComputable, ev.loaded, ev.total)
            reject(rd.error)
        })
        rd.addEventListener("loadstart", ev => {
            progress?.(ev.lengthComputable, ev.loaded, ev.total)
        })
        rd.addEventListener("progress", ev => {
            progress?.(ev.lengthComputable, ev.loaded, ev.total)
        })
        rd.addEventListener("load", () => {
            resolve(rd.result as ArrayBuffer)
        })
        rd.readAsArrayBuffer(blob)
    })
}

function get(url: string, progress?: (lengthComputable: boolean, loaded: number, total: number) => void): Promise<ArrayBuffer> {
    return new Promise((resolve, reject) => {
        const req = new XMLHttpRequest()
        req.addEventListener("abort", ev => {
            progress?.(ev.lengthComputable, ev.loaded, ev.total)
            reject(new Error("request aborted"))
        })
        req.addEventListener("error", ev => {
            progress?.(ev.lengthComputable, ev.loaded, ev.total)
            reject(new Error("network error"))
        })
        req.addEventListener("timeout", ev => {
            progress?.(ev.lengthComputable, ev.loaded, ev.total)
            reject(new Error("request timed out"))
        })
        req.addEventListener("load", ev => {
            progress?.(ev.lengthComputable, ev.loaded, ev.total)
            if (req.status != 200) {
                reject(new Error(`status ${req.status} ${req.statusText}`))
            } else {
                resolve(req.response)
            }
        })
        req.addEventListener("loadstart", ev => {
            progress?.(ev.lengthComputable, ev.loaded, ev.total)
        })
        req.addEventListener("progress", ev => {
            progress?.(ev.lengthComputable, ev.loaded, ev.total)
        })
        req.responseType = "arraybuffer"
        req.open("GET", url)
        req.send()
    })
}

function formatBytes(bytes: number): string {
    if (bytes > 1000000) {
        return `${Math.round(bytes/1000000)} MB`
    } else if (bytes > 1000) {
        return `${Math.round(bytes/1000)} KB`
    } else {
        return `${Math.round(bytes)} bytes`
    }
}
