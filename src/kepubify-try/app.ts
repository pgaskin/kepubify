/* eslint-disable @typescript-eslint/no-non-null-assertion */
import * as Sentry from "@sentry/minimal"
import { Kepubify, KepubifyConfig } from "../kepubify"

export class KepubifyApp {
    private kepubify: Kepubify // private instead of actually private to allow it to be accessed from devtools
    #version?: string
    #ref: {
        el: HTMLElement,
        title: HTMLElement,
        files: HTMLElement,
        options: HTMLFormElement,
        placeholder?: HTMLElement,
    }
    #initializedUI = false

    constructor(id: string) {
        const el = document.querySelector(id)! as HTMLElement
        this.kepubify = new Kepubify()
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
            this.#version = await this.kepubify.init((lengthComputable, loaded, total) => {
                if (lengthComputable) {
                    this.#ref.title.textContent = `Downloading kepubify... ${(loaded / 1000000).toFixed(2)} / ${(total / 1000000).toFixed(2)} MB (${(loaded / total * 100).toFixed(0)}%)`
                } else if (loaded) {
                    this.#ref.title.textContent = `Downloading kepubify... ${(loaded / 1000000).toFixed(2)} MB`
                } else {
                    this.#ref.title.textContent = "Downloading kepubify..."
                }
            })
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

        this.#ref.placeholder = el

        this.#ref.placeholder.addEventListener("click", () => {
            const input = document.createElement("input")
            input.type = "file"
            input.accept = "application/epub+zip"
            input.multiple = true
            input.addEventListener("input", () => {
                if (input.files) {
                    for (let i = 0; i < input.files.length; i++) {
                        this.enqueue(input.files[i])
                    }
                }
            })
            input.click()
        })

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

        let kepub, config
        try {
            config = this.config
            kepub = await this.kepubify.kepubify(epub, config, (n, total) => {
                elInfoProgress.style.width = `${(total ? n / total : 0) * 100}%`
            })
            elInfoProgress.style.width = "100%"
            elInfoProgress.classList.add("file__info__progress--success")
        } catch (ex) {
            elInfoProgress.style.width = "100%"
            elInfoProgress.classList.add("file__info__progress--error")

            const elStatus = el.appendChild(document.createElement("div"))
            elStatus.classList.add("file__status")
            elStatus.textContent = ex.toString()

            const elInfoRemove = elInfo.appendChild(document.createElement("button"))
            elInfoRemove.textContent = "−"
            elInfoRemove.title = "Remove"
            elInfoRemove.addEventListener("click", () => {
                el.remove()
            })

            Sentry.captureException(ex, {
                extra: {
                    file: { name: epub?.name, size: epub?.size, type: epub?.type },
                    worker: { version: this.#version },
                    config: config,
                },
            })
            return
        }

        Sentry.addBreadcrumb({
            message: `Finished ${epub.name} (${kepub.size} bytes)`,
        })

        try {
            elInfoName.href = URL.createObjectURL(new Blob([kepub], { type: "application/epub+zip" }))
            elInfoName.download = epub.name.replace(/([.]epub)?$/, ".kepub.epub")
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
                    kepub: { size: kepub.size },
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

    get config(): KepubifyConfig {
        const cfg = new KepubifyConfig()
        switch (this.#ref.options["smarten-punctuation"].value) {
            case "off":
                break
            case "on":
                cfg.smartypants()
                break
        }
        switch (this.#ref.options["dummy-titlepage"].value) {
            case "auto":
                break
            case "always":
                cfg.dummyTitlepage(true)
                break
            case "never":
                cfg.dummyTitlepage(false)
                break
        }
        if (this.#ref.options["custom-css"].value.trim() != "") {
            cfg.addCSS(this.#ref.options["custom-css"].value)
        }
        switch (this.#ref.options["hyphenation"].value) {
            case "default":
                break
            case "enable":
                cfg.hyphenate(true)
                break
            case "disable":
                cfg.hyphenate(false)
                break
        }
        switch (this.#ref.options["full-screen-fixes"].value) {
            case "off":
                break
            case "on":
                cfg.fullScreenFixes()
                break
        }
        return cfg
    }
}
