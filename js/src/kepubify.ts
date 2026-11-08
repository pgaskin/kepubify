/** Options for {@link convert} and {@link transformContent}. */
export interface Options {
	/**
	 * The charset of the content documents. If empty or "auto", it is detected
	 * from the content. If the input of {@link transformContent} is a string,
	 * it defaults to "utf-8".
	 */
	charset?: string;

	/**
	 * CSS to add in new style elements in the content documents.
	 */
	css?: string[];

	/**
	 * Force-enables (except in headings and table cells) or force-disables
	 * hyphenation. If unset, it is left to the book and reader styling.
	 */
	hyphenate?: boolean;

	/**
	 * Applies fixes for fullscreen layout on firmware older than 4.19.11911.
	 */
	fullScreenFixes?: boolean;

	/**
	 * Converts punctuation to smart punctuation.
	 */
	smartypants?: boolean;

	/**
	 * Raw strings to replace in the transformed HTML, in order. Replacements
	 * with an empty find string are ignored.
	 */
	replace?: { find: string; replace: string }[];

	/**
	 * Force-enables or force-disables adding a dummy titlepage to fix layout
	 * issues in some books. If unset, a heuristic is used.
	 */
	dummyTitlepage?: boolean;

	/**
	 * Converts books which are already KEPUBs instead of throwing {@link AlreadyKepubError}.
	 */
	reprocessKepub?: boolean;

	/**
	 * Shows the book's cover image on the dummy titlepage, if found.
	 */
	detectCover?: boolean;

	/**
	 * Called for content which can't be converted exactly, with the path of the
	 * file in the EPUB (empty if not applicable). Throwing aborts the
	 * conversion.
	 */
	warn?: (name: string, msg: string) => void;

	/**
	 * Called by {@link convert} as the conversion progresses, from 0 to 1.
	 * Throwing aborts the conversion.
	 */
	progress?: (percent: number) => void;
}

/** Thrown by {@link convert} when the book is already a KEPUB and {@link Options.reprocessKepub} is not set. */
export class AlreadyKepubError extends Error {
	constructor(message: string) {
		super(message);
		this.name = "AlreadyKepubError";
	}
}

/** A message from the calling thread to the worker. */
export interface Request {
	module: WebAssembly.Module;
	fn: "convert" | "transform_content";
	input: Uint8Array;
	options: Omit<Options, "warn" | "progress"> & { warn: boolean; progress: boolean };
}

/** A message from the worker to the calling thread. */
export type Response =
	| { progress: number }
	| { warn: [name: string, msg: string] }
	| { output: Uint8Array<ArrayBuffer> }
	| { error: string; alreadyKepub: boolean };

interface Exports {
	memory: WebAssembly.Memory;
	_initialize(): void;
	alloc(n: number): number;
	convert(input: number, inputLen: number, options: number, optionsLen: number): number;
	transform_content(input: number, inputLen: number, options: number, optionsLen: number): number;
}

let compiled: Promise<WebAssembly.Module> | undefined;

/**
 * Sets the compiled module to use instead of loading kepubify.wasm from beside
 * this module.
 */
export function setModule(module: WebAssembly.Module | PromiseLike<WebAssembly.Module>) {
	compiled = Promise.resolve(module);
}

/** Returns the compiled module, loading it if needed. */
export function getModule(): Promise<WebAssembly.Module> {
	return compiled ??= compile(new URL("kepubify.wasm", import.meta.url));
}

export const enc = new TextEncoder(), dec = new TextDecoder();

/**
 * Returns the input of transformContent as bytes, with the charset defaulting
 * to UTF-8 if it is a string.
 */
export function contentInput(html: string | Uint8Array, options: Options): [Uint8Array, Options] {
	if (typeof html == "string") {
		return [enc.encode(html), { ...options, charset: options.charset || "utf-8" }];
	}
	return [html, options];
}

async function compile(url: URL) {
	// fetch doesn't support file URLs in node (nodejs/node#48554)
	if (url.protocol == "file:" && typeof process != "undefined" && process.versions?.node) {
		// @ts-expect-error no node types
		const { readFile } = await import("node:fs/promises");
		// @ts-expect-error no node types
		const { fileURLToPath } = await import("node:url");
		return WebAssembly.compile(await readFile(fileURLToPath(url)));
	}
	return WebAssembly.compileStreaming(fetch(url));
}

declare const process: { versions?: { node?: string } } | undefined;

const flags = [
	"fullScreenFixes",
	"smartypants",
	"detectCover",
	"reprocessKepub",
	"warn",
	"progress",
] as const;

function encodeOptions(o: Options) {
	const parts: Uint8Array[] = [];
	const u32 = (v: number) => {
		const b = new Uint8Array(4);
		new DataView(b.buffer).setUint32(0, v, true);
		parts.push(b);
	};
	const str = (s: string) => {
		const b = enc.encode(s);
		u32(b.length);
		parts.push(b);
	};
	const tri = (v?: boolean) => v === undefined ? 0 : v ? 1 : 2;
	parts.push(new Uint8Array([
		flags.reduce((v, k, i) => v | (o[k] ? 1 << i : 0), 0),
		tri(o.hyphenate),
		tri(o.dummyTitlepage),
	]));
	str(o.charset ?? "");
	u32(o.css?.length ?? 0);
	for (const s of o.css ?? []) str(s);
	u32(o.replace?.length ?? 0);
	for (const r of o.replace ?? []) str(r.find), str(r.replace);

	const buf = new Uint8Array(parts.reduce((n, b) => n + b.length, 0));
	let p = 0;
	for (const b of parts) buf.set(b, p), p += b.length;
	return buf;
}

async function instantiate(module: WebAssembly.Module, o: Options) {
	let x!: Exports, dv: DataView, output = "";
	const view = () => dv?.buffer == x.memory.buffer ? dv : (dv = new DataView(x.memory.buffer));
	const bytes = (p: number, n: number) => new Uint8Array(x.memory.buffer, p, n);

	const wasi: Record<string, (...args: any[]) => number> = {};
	const ENOSYS = 52, EBADF = 8;
	for (const { module: m, name } of WebAssembly.Module.imports(module)) {
		if (m == "wasi_snapshot_preview1") wasi[name] = () => ENOSYS;
		else if (m != "kepubify") throw new Error(`unexpected import ${m}.${name}`);
	}
	wasi.args_sizes_get = wasi.environ_sizes_get = (n, size) => (view().setUint32(n, 0, true), view().setUint32(size, 0, true), 0); // empty args/env
	wasi.fd_prestat_get = () => EBADF; // no opened dirs
	wasi.fd_write = (fd, iovs, iovsLen, nwritten) => {
		if (fd != 1 && fd != 2) return EBADF;
		let n = 0;
		for (let i = 0; i < iovsLen; i++) {
			const p = view().getUint32(iovs + i * 8, true), l = view().getUint32(iovs + i * 8 + 4, true);
			output += dec.decode(bytes(p, l));
			n += l;
		}
		view().setUint32(nwritten, n, true);
		return 0;
	};
	wasi.clock_time_get = (_id, _precision, out) => (view().setBigUint64(out, BigInt(Math.round(performance.now() * 1e6)), true), 0);
	wasi.random_get = (p, n) => (crypto.getRandomValues(bytes(p, n)), 0);
	wasi.proc_exit = code => { throw new Error(`kepubify: wasm exited with code ${code}${output ? `: ${output.trim()}` : ""}`); };

	const kepubify = {
		warn: (name: number, nameLen: number, msg: number, msgLen: number) => o.warn?.(dec.decode(bytes(name, nameLen)), dec.decode(bytes(msg, msgLen))),
		progress: (percent: number) => o.progress?.(percent),
	};

	x = (await WebAssembly.instantiate(module, { wasi_snapshot_preview1: wasi, kepubify })).exports as unknown as Exports;
	x._initialize();
	return { x, view, bytes };
}

/** Calls the export fn of a new instance of module. */
export async function call(module: WebAssembly.Module, fn: "convert" | "transform_content", input: Uint8Array, o: Options): Promise<Uint8Array<ArrayBuffer>> {
	const { x, view, bytes } = await instantiate(module, o);
	const opts = encodeOptions(o);
	const ip = x.alloc(input.length);
	bytes(ip, input.length).set(input);
	const op = x.alloc(opts.length);
	bytes(op, opts.length).set(opts);
	const r = x[fn](ip, input.length, op, opts.length);
	const [status, p, n] = [0, 4, 8].map(i => view().getUint32(r + i, true));
	const out = bytes(p, n).slice();
	switch (status) {
		case 0:
			return out;
		case 2:
			throw new AlreadyKepubError(dec.decode(out));
		default:
			throw new Error(`kepubify: ${dec.decode(out)}`);
	}
}
