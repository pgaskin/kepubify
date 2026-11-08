/* eslint-disable @typescript-eslint/no-non-null-assertion */
import * as Sentry from "@sentry/browser"
import { AlreadyKepubError, convert, setModule, type WorkerOptions } from "kepubify/worker"

declare const KEPUBIFY_VERSION: string

const maxFileSize = 300_000_000
const maxConversions = Math.min(navigator.hardwareConcurrency || 2, 4)

export class KepubifyApp {
    #version = KEPUBIFY_VERSION
    #running = 0
    #waiting: (() => void)[] = []
    #ref: {
        el: HTMLElement,
        title: HTMLElement,
        files: HTMLElement,
        options: HTMLFormElement,
        placeholder?: HTMLElement,
        hint?: HTMLElement,
    }
    #initializedUI = false

    constructor(id: string) {
        const el = document.querySelector(id)! as HTMLElement
        this.#ref = {
            el,
            title: el.querySelector(".try__pane--files > .try__pane__title")!,
            files: el.querySelector(".try__pane--files > .try__pane__contents")!,
            options: el.querySelector(".try__pane--options > .try__pane__contents > form.options")!,
        }
    }

    async start(): Promise<void> {
        if (this.#initializedUI) {
            return
        }

        Sentry.addBreadcrumb({
            message: "Initializing kepubify",
        })

        this.#ref.title.textContent = "Downloading kepubify..."
        try {
            const module = WebAssembly.compile(await download(new URL("kepubify.wasm", import.meta.url), (loaded, total) => {
                if (total) {
                    this.#ref.title.textContent = `Downloading kepubify... ${(loaded / 1000000).toFixed(2)} / ${(total / 1000000).toFixed(2)} MB (${(loaded / total * 100).toFixed(0)}%)`
                } else if (loaded) {
                    this.#ref.title.textContent = `Downloading kepubify... ${(loaded / 1000000).toFixed(2)} MB`
                } else {
                    this.#ref.title.textContent = "Downloading kepubify..."
                }
            }))
            setModule(module)
            await module
        } catch (ex) {
            this.#ref.title.textContent = `Failed to download kepubify: ${ex}`
            this.#ref.title.title = `${ex}`
            throw new Error(`failed to initialize kepubify: ${ex}`)
        }
        this.#ref.title.textContent = `Kepubify ${this.#version}`

        Sentry.addBreadcrumb({
            message: `Initialized kepubify ${this.#version}`,
        })

        const el = this.#ref.files.appendChild(document.createElement("button"))
        el.classList.add("file")
        el.classList.add("file--placeholder")
        el.title = "Browse"

        const elInfo = el.appendChild(document.createElement("div"))
        elInfo.classList.add("file__info")

        const elInfoName = elInfo.appendChild(document.createElement("span"))
        elInfoName.classList.add("file__info__name")
        elInfoName.textContent = window.matchMedia("(pointer: coarse)").matches
            ? "Choose a file..."
            : "Drop your files here, or click to browse..."

        this.#ref.hint = this.#ref.files.appendChild(document.createElement("div"))
        this.#ref.hint.classList.add("hint")
        this.#ref.hint.classList.add("hint--hidden")
        this.#ref.hint.textContent = "Each file will turn green when the conversion is complete. Click the blue filename link to download the converted file."

        this.#ref.placeholder = el

        // note: in the document since iOS Safari may not report the selected
        // files for a detached input
        const input = this.#ref.files.appendChild(document.createElement("input"))
        input.type = "file"
        input.accept = ".epub,application/epub+zip" // note: Android often has no MIME type for epubs
        input.multiple = true
        input.hidden = true
        input.addEventListener("change", () => {
            if (input.files) {
                for (let i = 0; i < input.files.length; i++) {
                    this.enqueue(input.files[i])
                }
            }
            input.value = "" // so the same files can be selected again
        })
        this.#ref.placeholder.addEventListener("click", () => input.click())

        for (const x of ["drag", "dragstart", "dragend", "dragover", "dragenter", "dragleave", "drop"]) {
            this.#ref.files.addEventListener(x, ev => {
                if (x == "drop") {
                    const files = (ev as DragEvent).dataTransfer?.files
                    if (files) {
                        for (let i = 0; i < files.length; i++) {
                            this.enqueue(files[i])
                        }
                    }
                }
                if (x == "dragover" || x == "dragenter") {
                    this.#ref.files.classList.add("dragover")
                }
                if (x == "dragend" || x == "dragleave" || x == "drop") {
                    this.#ref.files.classList.remove("dragover")
                }
                ev.preventDefault()
                ev.stopPropagation()
            })
        }

        this.#initializedUI = true
    }

    async enqueue(epub: File): Promise<void> {
        Sentry.addBreadcrumb({
            message: `Enqueuing ${epub.name} (size=${epub.size}, type=${epub.type})`,
        })

        this.#ref.hint!.classList.remove("hint--hidden")

        const el = this.#ref.placeholder!.insertAdjacentElement("beforebegin", document.createElement("div"))!
        el.classList.add("file")
        el.setAttribute("tabindex", "0")

        const elInfo = el.appendChild(document.createElement("div"))
        elInfo.classList.add("file__info")

        const elInfoProgress = elInfo.appendChild(document.createElement("span"))
        elInfoProgress.classList.add("file__info__progress")

        const elInfoName = elInfo.appendChild(document.createElement("a"))
        elInfoName.classList.add("file__info__name")
        elInfoName.textContent = epub.name

        let kepub: Uint8Array<ArrayBuffer>, config: WorkerOptions | undefined
        try {
            if (epub.size > maxFileSize) {
                throw new Error(`File too large for web version (maximum: ${maxFileSize / 1000000} MB)`)
            }
            config = this.config
            kepub = await this.#limit(async () => convert(new Uint8Array(await epub.arrayBuffer()), {
                ...config,
                progress: percent => {
                    elInfoProgress.style.width = `${percent * 100}%`
                },
            }))
            elInfoProgress.style.width = "100%"
            elInfoProgress.classList.add("file__info__progress--success")
        } catch (ex) {
            elInfoProgress.style.width = "100%"
            elInfoProgress.classList.add("file__info__progress--error")

            const elStatus = el.appendChild(document.createElement("div"))
            elStatus.classList.add("file__status")
            elStatus.textContent = ex instanceof AlreadyKepubError ? "Error: already a KEPUB" : `${ex}`

            const elInfoRemove = elInfo.appendChild(document.createElement("button"))
            elInfoRemove.textContent = "−"
            elInfoRemove.title = "Remove"
            elInfoRemove.addEventListener("click", () => {
                el.remove()
            })

            if (!(ex instanceof AlreadyKepubError)) {
                Sentry.captureException(ex, {
                    extra: {
                        file: { name: epub?.name, size: epub?.size, type: epub?.type },
                        worker: { version: this.#version },
                        config: config,
                    },
                })
            }
            return
        }

        Sentry.addBreadcrumb({
            message: `Finished ${epub.name} (${kepub.length} bytes)`,
        })

        try {
            elInfoName.href = URL.createObjectURL(new Blob([kepub], { type: "application/epub+zip" }))
            elInfoName.download = epub.name.replace(/([.]epub)?$/i, ".kepub.epub")
            elInfoName.textContent = elInfoName.download

            const elInfoRemove = elInfo.appendChild(document.createElement("button"))
            elInfoRemove.textContent = "−"
            elInfoRemove.title = "Remove"
            elInfoRemove.addEventListener("click", () => {
                el.remove()
                URL.revokeObjectURL(elInfoName.href)
            })
        } catch (ex) {
            elInfoProgress.style.width = "100%"
            elInfoProgress.classList.add("file__info__progress--error")

            const elStatus = el.appendChild(document.createElement("div"))
            elStatus.classList.add("file__status")
            elStatus.textContent = `Error: failed to save file: ${ex}`

            Sentry.captureException(new Error(`failed to create download link: ${ex}`), {
                extra: {
                    file: { name: epub.name, size: epub.size, type: epub.type },
                    kepub: { size: kepub.length },
                    worker: { version: this.#version },
                },
            })
            return
        }

        (window as any).goatcounter?.count({
            path: "kepubify-wasm-convert",
            event: true,
        })
    }

    get config(): WorkerOptions {
        const cfg: WorkerOptions = {}
        const option = (name: string) => (this.#ref.options.elements.namedItem(name) as HTMLInputElement | RadioNodeList).value
        switch (option("smarten-punctuation")) {
            case "off":
                break
            case "on":
                cfg.smartypants = true
                break
        }
        switch (option("dummy-titlepage")) {
            case "auto":
                break
            case "always":
                cfg.dummyTitlepage = true
                break
            case "never":
                cfg.dummyTitlepage = false
                break
        }
        if (option("custom-css").trim() != "") {
            cfg.css = [option("custom-css")]
        }
        switch (option("hyphenation")) {
            case "default":
                break
            case "enable":
                cfg.hyphenate = true
                break
            case "disable":
                cfg.hyphenate = false
                break
        }
        switch (option("full-screen-fixes")) {
            case "off":
                break
            case "on":
                cfg.fullScreenFixes = true
                break
        }
        return cfg
    }

    async #limit<T>(fn: () => Promise<T>): Promise<T> {
        while (this.#running >= maxConversions) {
            await new Promise<void>(resolve => this.#waiting.push(resolve))
        }
        this.#running++
        try {
            return await fn()
        } finally {
            this.#running--
            this.#waiting.shift()?.()
        }
    }
}

async function download(url: URL, progress: (loaded: number, total: number) => void): Promise<ArrayBuffer> {
    const resp = await fetch(url)
    if (!resp.ok) {
        throw new Error(`fetch ${url}: response status ${resp.status}`)
    }
    const total = Number(resp.headers.get("Content-Length")) || 0
    const chunks: Uint8Array[] = []
    let loaded = 0
    progress(loaded, total)
    const reader = resp.body!.getReader() // note: async iteration needs Safari 27
    for (let r; !(r = await reader.read()).done;) {
        chunks.push(r.value)
        progress(loaded += r.value.length, total)
    }
    const buf = new Uint8Array(loaded)
    let p = 0
    for (const chunk of chunks) {
        buf.set(chunk, p)
        p += chunk.length
    }
    return buf.buffer
}
