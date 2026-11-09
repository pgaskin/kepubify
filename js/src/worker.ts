import { type Options, type Request, type Response, AlreadyKepubError, contentInput, dec, getModule } from "./kepubify";

export { AlreadyKepubError, setModule } from "./kepubify";

/**
 * Options for {@link convert} and {@link transformContent}. The callbacks are
 * called on the main thread (i.e., not within the worker), and throwing from
 * them aborts the conversion like the AbortSignal.
 */
export interface WorkerOptions extends Options {
	/** Aborts the conversion, terminating the worker. */
	signal?: AbortSignal;
}

let createWorker = () => {
	if (typeof Worker == "undefined") {
		throw new Error("kepubify: Web Workers are not available (use the main module instead)");
	}
	return new Worker(new URL("./kepubify.worker.js", import.meta.url), { type: "module" });
};

/**
 * Sets the function used to create the workers instead of loading
 * kepubify.worker.js from beside this module as a module worker (e.g., to load
 * it from a Blob URL).
 */
export function setWorker(create: () => Worker) {
	createWorker = create;
}

/** Converts an EPUB to a KEPUB. */
export async function convert(epub: Uint8Array, options: WorkerOptions = {}): Promise<Uint8Array<ArrayBuffer>> {
	return run("convert", epub, options);
}

/** Transforms a single (X)HTML content document, returning it as UTF-8. */
export async function transformContent(html: string | Uint8Array, options: WorkerOptions = {}): Promise<string> {
	const [input, o] = contentInput(html, options);
	return dec.decode(await run("transform_content", input, o));
}

async function run(fn: Request["fn"], input: Uint8Array, o: WorkerOptions): Promise<Uint8Array<ArrayBuffer>> {
	const { signal, warn, progress, memory, ...options } = o;
	signal?.throwIfAborted();
	const module = await getModule();
	signal?.throwIfAborted();
	const w = createWorker();
	return new Promise((resolve, reject) => {
		const stop = (err?: unknown) => {
			w.terminate();
			signal?.removeEventListener("abort", abort);
			if (err !== undefined) reject(err);
		};
		const abort = () => stop(signal!.reason);
		signal?.addEventListener("abort", abort);
		w.onmessage = ({ data: m }: MessageEvent<Response>) => {
			try {
				if ("progress" in m) progress?.(m.progress);
				else if ("warn" in m) warn?.(...m.warn);
				else if ("memory" in m) memory?.(m.memory);
				else if ("output" in m) stop(), resolve(m.output);
				else stop(m.alreadyKepub ? new AlreadyKepubError(m.error) : new Error(m.error));
			} catch (err) {
				stop(err);
			}
		};
		w.onerror = e => (e.preventDefault(), stop(new Error(`kepubify: worker: ${e.message}`)));
		w.onmessageerror = () => stop(new Error("kepubify: worker: message can't be deserialized"));
		w.postMessage({
			module,
			fn,
			input,
			options: {
				...options,
				warn: !!warn,
				progress: !!progress,
				memory: !!memory,
			},
		} satisfies Request);
	});
}
