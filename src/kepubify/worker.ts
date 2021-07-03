/** @internal */
import "./wasm/wasm_exec.js"
import { KepubifyWasmInit } from "./wasm"

/** @internal */
declare global {
    class Go {
        constructor()
        argv: string[]
        importObject: WebAssembly.Imports
        run(instance: WebAssembly.Instance): Promise<void>
    }
    var init: KepubifyWasmInit | undefined
}

onmessage = async ev => {
    const go = new Go()
    const wasm = ev.data.wasm as WebAssembly.Module
    const instance = await WebAssembly.instantiate(wasm, go.importObject)
    go.argv = [(new Date().getTime()*1000 + Math.round(Math.random() * 1000)).toString(36)]
    if (((globalThis?? window ?? self)["init"] = ev.data.init as KepubifyWasmInit | undefined)) {
        go.argv.push("init")
    }
    await go.run(instance)
}
