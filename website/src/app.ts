/* eslint-disable @typescript-eslint/no-non-null-assertion */
import * as Sentry from "@sentry/browser"
import { AlreadyKepubError, convertBlob, setModule, type WorkerOptions } from "kepubify/worker"
import { zip, type ZipEntry } from "./zip"

declare const KEPUBIFY_VERSION: string

const maxFileSize = 300_000_000
const maxConversions = Math.min(navigator.hardwareConcurrency || 2, 4)

// note: iPadOS uses a macOS user agent by default
const isIOS = /\b(iPhone|iPad|iPod)\b/.test(navigator.userAgent) || /\bMacintosh\b/.test(navigator.userAgent) && navigator.maxTouchPoints > 1

const isCoarse = window.matchMedia("(pointer: coarse)").matches

const settingsKey = "kepubify-try-settings"
const maxWarnings = 500

export class KepubifyApp {
    #version = KEPUBIFY_VERSION
    #running = 0
    #waiting: (() => void)[] = []
    #books: Book[] = []
    #module?: Promise<WebAssembly.Module>
    #loaded = false
    #ref: {
        el: HTMLElement,
        settings: HTMLButtonElement,
        settingsSummary: HTMLElement,
        dialog: HTMLDialogElement,
        options: HTMLFormElement,
        replace: HTMLElement,
        replaceTemplate: HTMLTemplateElement,
        drop: HTMLElement,
        dropText: HTMLElement,
        input: HTMLInputElement,
        books: HTMLElement,
        booksSummary: HTMLElement,
        booksList: HTMLElement,
        downloadAll: HTMLButtonElement,
        shareHint: HTMLElement,
        next: HTMLElement,
        template: HTMLTemplateElement,
    }
    #initializedUI = false

    constructor(id: string) {
        const el = document.querySelector(id)! as HTMLElement
        this.#ref = {
            el,
            settings: el.querySelector(".try-settings")!,
            settingsSummary: el.querySelector(".try-settings__summary")!,
            dialog: el.querySelector(".try-dialog")!,
            options: el.querySelector(".try-dialog form.options")!,
            replace: el.querySelector(".try-dialog .replace")!,
            replaceTemplate: el.querySelector(".try-replace-template")!,
            drop: el.querySelector(".try-drop")!,
            dropText: el.querySelector(".try-drop__text")!,
            input: el.querySelector(".try-drop input[type=file]")!,
            books: el.querySelector(".try-books")!,
            booksSummary: el.querySelector(".try-books__summary")!,
            booksList: el.querySelector(".try-books__list")!,
            downloadAll: el.querySelector(".try-books__all")!,
            shareHint: el.querySelector(".try-share-hint")!,
            next: el.querySelector(".try-next")!,
            template: el.querySelector(".try-book-template")!,
        }
    }

    async start(): Promise<void> {
        if (this.#initializedUI) {
            return
        }
        this.#initUI()
        this.#initializedUI = true

        Sentry.addBreadcrumb({
            message: "Initializing kepubify",
        })

        this.#ref.dropText.textContent = "Downloading kepubify..."
        this.#module = download(new URL("kepubify.wasm", import.meta.url), (loaded, total) => {
            if (total) {
                this.#ref.dropText.textContent = `Downloading kepubify... ${(loaded / 1000000).toFixed(1)} / ${(total / 1000000).toFixed(1)} MB (${(loaded / total * 100).toFixed(0)}%)`
            } else if (loaded) {
                this.#ref.dropText.textContent = `Downloading kepubify... ${(loaded / 1000000).toFixed(1)} MB`
            }
        }).then(buf => WebAssembly.compile(buf))
        setModule(this.#module)
        try {
            await this.#module
        } catch (ex) {
            this.#ref.dropText.textContent = `Failed to download kepubify: ${ex}`
            this.#ref.dropText.classList.add("try-drop__text--error")
            throw new Error(`failed to initialize kepubify: ${ex}`)
        }
        this.#ref.drop.title = `Kepubify ${this.#version}`
        this.#loaded = true
        this.#update()

        Sentry.addBreadcrumb({
            message: `Initialized kepubify ${this.#version}`,
        })
    }

    enqueue(epub: File): void {
        Sentry.addBreadcrumb({
            message: `Enqueuing ${epub.name} (size=${epub.size}, type=${epub.type})`,
        })
        const book = new Book(this.#ref.template, epub, isIOS)
        book.onremove = () => {
            book.abort.abort()
            book.el.remove()
            book.revoke()
            this.#books.splice(this.#books.indexOf(book), 1)
            this.#update()
        }
        book.onupdate = () => this.#update()
        this.#books.push(book)
        this.#update()
        this.#convert(book, this.config)
    }

    get config(): WorkerOptions {
        const cfg: WorkerOptions = {}
        const el = this.#ref.options.elements
        const value = (name: string) => (el.namedItem(name) as HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement).value
        const checked = (name: string) => (el.namedItem(name) as HTMLInputElement).checked
        if (checked("smarten-punctuation")) {
            cfg.smartypants = true
        }
        switch (value("hyphenation")) {
            case "enable":
                cfg.hyphenate = true
                break
            case "disable":
                cfg.hyphenate = false
                break
        }
        switch (value("dummy-titlepage")) {
            case "auto-cover":
                cfg.detectCover = true
                break
            case "always":
                cfg.dummyTitlepage = true
                break
            case "always-cover":
                cfg.dummyTitlepage = true
                cfg.detectCover = true
                break
            case "never":
                cfg.dummyTitlepage = false
                break
        }
        if (value("charset") != "auto") {
            cfg.charset = value("charset")
        }
        if (checked("full-screen-fixes")) {
            cfg.fullScreenFixes = true
        }
        if (value("custom-css").trim() != "") {
            cfg.css = [value("custom-css")]
        }
        const replace = this.#replacements.filter(([find]) => find != "").map(([find, replace]) => ({ find, replace }))
        if (replace.length) {
            cfg.replace = replace
        }
        return cfg
    }

    #initUI() {
        this.#ref.settings.addEventListener("click", () => this.#ref.dialog.showModal())
        this.#ref.dialog.addEventListener("click", ev => {
            if (ev.target == this.#ref.dialog) {
                this.#ref.dialog.close()
            }
        })
        this.#replacements = []
        this.#loadSettings()
        this.#ref.options.addEventListener("input", () => this.#updateSettings(true))
        this.#ref.options.addEventListener("change", () => this.#updateSettings(true))
        this.#ref.options.addEventListener("reset", () => setTimeout(() => {
            this.#replacements = []
            this.#updateSettings(true)
        }))
        this.#ref.replace.addEventListener("keydown", ev => {
            if (ev.key == "Enter") {
                ev.preventDefault() // don't close the dialog
            }
        })
        this.#updateSettings(false)

        // note: in the document since iOS Safari may not report the selected
        // files for a detached input
        const input = this.#ref.input
        input.addEventListener("change", () => {
            if (input.files) {
                for (let i = 0; i < input.files.length; i++) {
                    this.enqueue(input.files[i])
                }
            }
            input.value = "" // so the same files can be selected again
        })
        this.#ref.drop.addEventListener("click", ev => {
            if (ev.target != input) {
                input.click()
            }
        })

        let depth = 0
        const hasFiles = (ev: DragEvent) => !!ev.dataTransfer?.types.includes("Files")
        document.addEventListener("dragenter", ev => {
            if (hasFiles(ev)) {
                depth++
                this.#ref.drop.classList.add("try-drop--dragover")
                ev.preventDefault()
            }
        })
        document.addEventListener("dragleave", ev => {
            if (hasFiles(ev) && --depth <= 0) {
                depth = 0
                this.#ref.drop.classList.remove("try-drop--dragover")
            }
        })
        document.addEventListener("dragover", ev => {
            if (hasFiles(ev)) {
                ev.dataTransfer!.dropEffect = "copy"
                ev.preventDefault()
            }
        })
        document.addEventListener("drop", ev => {
            if (hasFiles(ev)) {
                depth = 0
                this.#ref.drop.classList.remove("try-drop--dragover")
                ev.preventDefault()
                const files = ev.dataTransfer!.files
                for (let i = 0; i < files.length; i++) {
                    this.enqueue(files[i])
                }
            }
        })

        this.#ref.downloadAll.addEventListener("click", () => this.#downloadAll())
        this.#update()
    }

    async #convert(book: Book, config: WorkerOptions) {
        let kepub: Blob, start = 0, memory = 0
        const log = (result: string, err?: unknown) => {
            let msg = `kepubify: ${book.input.name}: ${result}`
            if (start) {
                msg += ` in ${Math.round(performance.now() - start)} ms`
            }
            if (memory) {
                msg += `, peak memory ${formatSize(memory)}`
            }
            if (err) {
                msg += `: ${err}`
            }
            console.log(msg)
        }
        try {
            if (book.input.size > maxFileSize) {
                throw new Error(`File too large for the web version (maximum: ${maxFileSize / 1000000} MB)`)
            }
            await this.#module
            kepub = await this.#limit(async () => {
                book.abort.signal.throwIfAborted()
                book.converting(0)
                start = performance.now()
                return convertBlob(book.input, {
                    ...config,
                    signal: book.abort.signal,
                    progress: percent => book.converting(percent),
                    warn: (name, msg) => book.warnings.push([name, msg]),
                    memory: bytes => memory = bytes,
                })
            })
        } catch (ex) {
            if (book.abort.signal.aborted) {
                return
            }
            if (ex instanceof AlreadyKepubError) {
                log(`${formatSize(book.input.size)}, already a kepub`)
                book.kepub(() => {
                    book.reset()
                    this.#convert(book, { ...config, reprocessKepub: true })
                })
                return
            }
            log(`${formatSize(book.input.size)}, failed`, ex)
            book.failed(ex)
            Sentry.captureException(ex, {
                extra: {
                    file: { name: book.input.name, size: book.input.size, type: book.input.type },
                    worker: { version: this.#version },
                    config: config,
                },
            })
            return
        }

        log(`${formatSize(book.input.size)} -> ${formatSize(kepub.size)}`)
        Sentry.addBreadcrumb({
            message: `Finished ${book.input.name} (${kepub.size} bytes)`,
        })

        try {
            const name = book.input.name.replace(/([.]epub)?$/i, ".kepub.epub")
            const crc = (kepub as Blob & Record<symbol, unknown>)[Symbol.for("kepubify.crc32")] // kepubify.ts crc32Key
            if (typeof crc != "number") {
                throw new Error("missing crc32")
            }
            const file = new File([kepub], name, { type: "application/epub+zip" })
            book.ready(file, crc)
        } catch (ex) {
            book.failed(new Error(`Failed to save file: ${ex}`))
            Sentry.captureException(new Error(`failed to create download link: ${ex}`), {
                extra: {
                    file: { name: book.input.name, size: book.input.size, type: book.input.type },
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

    #update() {
        this.#sort()

        const count = (state: BookState) => this.#books.filter(b => b.state == state).length
        const ready = count("ready")

        this.#ref.books.classList.toggle("hidden", this.#books.length == 0)
        this.#ref.booksSummary.replaceChildren()
        this.#ref.booksSummary.appendChild(document.createElement("b")).textContent = `${ready} ready`
        for (const [n, what] of [
            [count("converting"), "converting"],
            [count("waiting"), "waiting"],
            [count("failed"), "failed"],
        ] as const) {
            if (n) {
                this.#ref.booksSummary.append(` · ${n} ${what}`)
            }
        }

        this.#ref.downloadAll.classList.toggle("hidden", ready == 0)
        this.#ref.downloadAll.textContent = isIOS ? "Save all" : "Download all (.zip)"
        this.#ref.shareHint.classList.toggle("hidden", !isIOS || ready == 0)
        this.#ref.next.classList.toggle("hidden", ready == 0)

        const more = this.#books.length != 0
        if (this.#loaded) {
            this.#ref.dropText.textContent = isCoarse
                ? more ? "Add more EPUB files" : "Tap to choose EPUB files to convert"
                : more ? "Drop more EPUB files anywhere on this page" : "Drop EPUB files anywhere on this page, or click to open"
        }
        this.#ref.drop.classList.toggle("try-drop--empty", !more)
    }

    #loadSettings() {
        let saved: Record<string, unknown>
        try {
            saved = JSON.parse(localStorage.getItem(settingsKey) ?? "{}")
        } catch {
            return
        }
        if (Array.isArray(saved.replace)) {
            this.#replacements = saved.replace.filter(r => Array.isArray(r) && r.length == 2 && r.every(x => typeof x == "string"))
        }
        for (const el of formFields(this.#ref.options)) {
            const v = saved[el.name]
            if (el instanceof HTMLInputElement && el.type == "checkbox") {
                if (typeof v == "boolean") {
                    el.checked = v
                }
            } else if (typeof v == "string") {
                el.value = v
                if (el instanceof HTMLSelectElement && el.selectedIndex == -1) {
                    el.selectedIndex = 0 // removed option
                }
            }
        }
    }

    #saveSettings() {
        const saved: Record<string, unknown> = {}
        for (const el of formFields(this.#ref.options)) {
            saved[el.name] = el instanceof HTMLInputElement && el.type == "checkbox" ? el.checked : el.value
        }
        saved.replace = this.#replacements.filter(([find, replace]) => find != "" || replace != "")
        try {
            localStorage.setItem(settingsKey, JSON.stringify(saved))
        } catch {
            // private browsing or storage disabled
        }
    }

    #sort() {
        const stateOrder: Record<BookState, number> = {
            // ascending by convert start time
            converting: 0,
            // descending by added/completed
            waiting: 1,
            kepub: 2,
            ready: 3,
            failed: 3,
        }
        this.#books.sort((a, b) =>
            stateOrder[a.state] - stateOrder[b.state] ||
            (a.state == "converting"
                ? a.since - b.since
                : b.since - a.since))
        const list = this.#ref.booksList
        const focus = list.contains(document.activeElement) ? document.activeElement as HTMLElement : null
        let moved = false
        this.#books.forEach((book, i) => {
            if (list.children[i] != book.el) {
                list.insertBefore(book.el, list.children[i] ?? null)
                moved = true
            }
        })
        if (moved && focus && document.activeElement != focus) {
            focus.focus({ preventScroll: true })
        }
    }

    #updateSettings(save: boolean) {
        if (save) {
            this.#saveSettings()
        }
        const cfg = this.config
        const parts: string[] = []
        if (cfg.smartypants) {
            parts.push("smarten punctuation")
        }
        if (cfg.hyphenate !== undefined) {
            parts.push(cfg.hyphenate ? "enable hyphenation" : "disable hyphenation")
        }
        if (cfg.dummyTitlepage !== undefined) {
            parts.push(cfg.dummyTitlepage ? "always add title page" : "no title page")
        }
        if (cfg.detectCover) {
            parts.push("detect cover image")
        }
        if (cfg.charset) {
            parts.push(`${cfg.charset == "utf-8" ? "UTF-8" : cfg.charset == "windows-1252" ? "Windows-1252" : cfg.charset} text`)
        }
        if (cfg.fullScreenFixes) {
            parts.push("full-screen fixes")
        }
        if (cfg.css) {
            parts.push("custom CSS")
        }
        if (cfg.replace) {
            parts.push(`${cfg.replace.length} ${cfg.replace.length == 1 ? "replacement" : "replacements"}`)
        }
        const summary = parts.join(", ")
        this.#ref.settingsSummary.textContent = summary ? summary[0].toUpperCase() + summary.slice(1) : "Defaults"
    }

    get #replacements(): [string, string][] {
        return [...this.#ref.replace.children].map(row => [
            (row.querySelector(".replace__find") as HTMLInputElement).value,
            (row.querySelector(".replace__with") as HTMLInputElement).value,
        ])
    }

    set #replacements(rules: [string, string][]) {
        this.#ref.replace.replaceChildren()
        for (const [find, replace] of rules) {
            this.#addReplacement(find, replace)
        }
        this.#addReplacement()
    }

    #addReplacement(find = "", replace = "") {
        const row = this.#ref.replaceTemplate.content.firstElementChild!.cloneNode(true) as HTMLElement
        const [elFind, elWith] = row.querySelectorAll("input")
        elFind.value = find
        elWith.value = replace
        row.addEventListener("input", () => {
            if (row == this.#ref.replace.lastElementChild && (elFind.value || elWith.value)) {
                this.#addReplacement()
            }
        })
        row.querySelector(".replace__remove")!.addEventListener("click", () => {
            (row.nextElementSibling?.querySelector("input") as HTMLInputElement | null)?.focus()
            row.remove()
            this.#updateSettings(true)
        })
        this.#ref.replace.appendChild(row)
    }

    #downloadAll() {
        try {
            const names = new Set<string>()
            const entries = this.#books.flatMap(b => b.entry ? [b.entry] : []).map(e => {
                let name = e.name
                for (let i = 2; names.has(name); i++) {
                    name = e.name.replace(/([.]kepub[.]epub)?$/, ` (${i})$1`)
                }
                names.add(name)
                return { ...e, name }
            })
            save(new File([zip(entries)], "kepubify.zip", { type: "application/zip" }))
        } catch (ex) {
            this.#ref.downloadAll.textContent = `Error: ${ex}`
            Sentry.captureException(new Error(`failed to create zip: ${ex}`))
        }
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

type BookState = "waiting" | "converting" | "ready" | "kepub" | "failed"

// incremented whenever a book changes state, for sorting
let seq = 0

const stateIcons: Record<BookState, string> = {
    waiting: "schedule",
    converting: "progress_activity",
    ready: "check_circle",
    kepub: "info",
    failed: "error",
}

class Book {
    readonly input: File
    readonly el: HTMLLIElement
    readonly abort = new AbortController()
    state: BookState = "waiting"
    since = 0 // seq when the state was last changed
    entry?: ZipEntry
    warnings: [name: string, msg: string][] = []
    onremove?: () => void
    onupdate?: () => void

    #output?: File
    #url?: string
    #share: boolean
    #ref: {
        icon: SVGSVGElement,
        name: HTMLAnchorElement,
        size: HTMLElement,
        status: HTMLElement,
        remove: HTMLButtonElement,
        action: HTMLElement,
        download: HTMLButtonElement,
        warningsToggle: HTMLButtonElement,
        warnings: HTMLElement,
    }

    constructor(template: HTMLTemplateElement, input: File, share: boolean) {
        this.input = input
        this.#share = share
        this.el = (template.content.firstElementChild!.cloneNode(true) as HTMLLIElement)
        this.#ref = {
            icon: this.el.querySelector(".book__icon")!,
            name: this.el.querySelector(".book__name")!,
            size: this.el.querySelector(".book__size")!,
            status: this.el.querySelector(".book__status")!,
            remove: this.el.querySelector(".book__remove")!,
            action: this.el.querySelector(".book__action")!,
            download: this.el.querySelector(".book__download")!,
            warningsToggle: this.el.querySelector(".book__warnings-toggle")!,
            warnings: this.el.querySelector(".book__warnings")!,
        }
        this.#ref.name.textContent = input.name
        this.#ref.size.textContent = `· ${formatSize(input.size)}`
        this.#ref.download.textContent = share ? "Save" : "Download"
        this.#ref.download.addEventListener("click", () => this.#save())
        this.#ref.name.addEventListener("click", ev => {
            if (this.#share) {
                ev.preventDefault()
                this.#save()
            }
        })
        this.#ref.remove.addEventListener("click", () => this.onremove?.())
        this.#ref.warningsToggle.addEventListener("click", () => {
            const expanded = this.#ref.warningsToggle.getAttribute("aria-expanded") != "true"
            this.#ref.warningsToggle.setAttribute("aria-expanded", `${expanded}`)
            this.#ref.warnings.classList.toggle("hidden", !expanded)
        })
        this.#setState("waiting", "Waiting", "waiting")
    }

    converting(percent: number) {
        const pct = Math.round(percent * 100)
        if (this.state != "converting") {
            this.#setState("converting", "Converting", "")
            this.el.setAttribute("role", "progressbar")
            this.el.setAttribute("aria-label", `Converting ${this.input.name}`)
            this.el.setAttribute("aria-valuemin", "0")
            this.el.setAttribute("aria-valuemax", "100")
        }
        this.el.setAttribute("aria-valuenow", `${pct}`)
        this.el.style.setProperty("--progress", `${pct}%`)
        this.#ref.status.textContent = `converting, ${pct}%`
    }

    ready(output: File, crc: number) {
        this.#output = output
        this.#url = URL.createObjectURL(output)
        this.entry = { name: output.name, data: output, crc32: crc }
        this.#ref.name.textContent = output.name
        this.#ref.name.href = this.#url
        this.#ref.name.download = output.name
        this.#ref.size.textContent = `· ${formatSize(output.size)}`
        this.#ref.download.disabled = false
        if (this.warnings.length) {
            this.#showWarnings()
        }
        this.#setState("ready", this.warnings.length ? "Ready with warnings" : "Ready", "")
    }

    kepub(convert: () => void) {
        const btn = document.createElement("button")
        btn.type = "button"
        btn.className = "btn btn--outline"
        btn.textContent = "Convert"
        btn.title = "Convert it again anyway"
        btn.addEventListener("click", convert)
        this.#ref.action.replaceChildren(btn)
        this.#setState("kepub", "Already a KEPUB", "already a KEPUB")
    }

    reset() {
        this.#ref.action.replaceChildren(this.#ref.download)
        this.#setState("waiting", "Waiting", "waiting")
    }

    failed(err: unknown) {
        const msg = `${err}`.replace(/^Error: /, "").replace(/^kepubify: /, "")
        this.#ref.status.title = msg
        const el = document.createElement("span")
        el.className = "book__failed"
        el.textContent = "Failed"
        this.#ref.action.replaceChildren(el)
        this.#setState("failed", "Failed", msg)
    }

    revoke() {
        if (this.#url) {
            URL.revokeObjectURL(this.#url)
            this.#url = undefined
        }
    }

    #showWarnings() {
        const n = this.warnings.length
        this.el.classList.add("book--warnings")
        this.#ref.warningsToggle.textContent = `${n} ${n == 1 ? "warning" : "warnings"}`
        this.#ref.warningsToggle.classList.remove("hidden")
        const ul = this.#ref.warnings.querySelector("ul")!
        for (const [name, msg] of this.warnings.slice(0, maxWarnings)) {
            const li = ul.appendChild(document.createElement("li"))
            const code = li.appendChild(document.createElement("code"))
            code.textContent = code.title = name
            li.appendChild(document.createElement("span")).textContent = msg
        }
        if (n > maxWarnings) {
            ul.appendChild(document.createElement("li")).textContent = `and ${n - maxWarnings} more`
        }
    }

    #setState(state: BookState, label: string, status: string) {
        if (this.state == "converting" && state != "converting") {
            for (const attr of ["role", "aria-label", "aria-valuemin", "aria-valuemax", "aria-valuenow"]) {
                this.el.removeAttribute(attr)
            }
        }
        this.state = state
        this.since = ++seq
        this.el.dataset.state = state
        const icon = state == "ready" && this.warnings.length ? "warning" : stateIcons[state]
        this.#ref.icon.querySelector("use")!.setAttribute("href", `#icon-${icon}`)
        this.#ref.icon.setAttribute("aria-label", label)
        this.#ref.status.textContent = status
        this.#ref.remove.setAttribute("aria-label", `${state == "converting" ? "Cancel" : "Remove"} ${this.input.name}`)
        this.onupdate?.()
    }

    #save() {
        if (this.#output) {
            save(this.#output, this.#share, this.#url)
        }
    }
}

function formFields(form: HTMLFormElement): (HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement)[] {
    return [...form.elements].filter(el => (el instanceof HTMLInputElement || el instanceof HTMLSelectElement || el instanceof HTMLTextAreaElement) && el.name) as (HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement)[]
}

// save downloads the file, or opens the share sheet if share is true and the
// browser supports it (iOS Safari opens downloaded epubs in Books, and some
// in-app browsers can't download files at all).
function save(file: File, share = isIOS, url?: string) {
    if (share && navigator.canShare?.({ files: [file] })) {
        navigator.share({ files: [file] }).catch(ex => {
            if (!(ex instanceof DOMException && ex.name == "AbortError")) {
                Sentry.captureException(new Error(`failed to share file: ${ex}`))
                download()
            }
        })
        return
    }
    download()

    function download() {
        const a = document.createElement("a")
        a.href = url ?? URL.createObjectURL(file)
        a.download = file.name
        a.click()
        if (!url) {
            setTimeout(() => URL.revokeObjectURL(a.href), 60_000)
        }
    }
}

function formatSize(n: number): string {
    return n < 1_000_000 ? `${Math.max(1, Math.round(n / 1000))} KB` : `${(n / 1_000_000).toFixed(1)} MB`
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
