declare module "inline:kepubify.wasm" {
    const wasm: Uint8Array<ArrayBuffer>
    export default wasm
}

declare module "inline:kepubify.worker" {
    const source: string
    export default source
}
