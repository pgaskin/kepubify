/** @internal */
export const KepubifyWasmURL = new URL("./wasm/kepubify.wasm", import.meta.url).toString()

/** @internal */
export interface KepubifyWasmInit {
    epub: ArrayBuffer,
    config: KepubifyWasmOption[],
    progress: boolean,
}

/** @internal */
export type KepubifyWasmOption = (
    { type: "Smartypants" } |
    { type: "FindReplace", find: string, replace: string } |
    { type: "DummyTitlepage", add: boolean } |
    { type: "AddCSS", css: string } |
    { type: "Hyphenate", hyphenate: boolean } |
    { type: "FullScreenFixes" }
)

/** @internal */
export type KepubifyWasmMessage = (
    /** initialization */
    { type: "version", version: string } |

    /** conversion */
    { type: "progress", n: number, total: number } |
    { type: "success", kepub: ArrayBuffer } |
    { type: "error", message: string } |

    /** exit */
    { type: "panic", message: string, stack: string } |
    { type: "exit" }
)
