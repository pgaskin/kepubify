// The main entry point, converting on the calling thread. See worker.ts for
// the same API running in a Web Worker.

import { type Input, type Options, call, callBlob, contentInput, dec, getModule, withCRC32 } from "./kepubify";

export { type Options, AlreadyKepubError, setModule, version, revision } from "./kepubify";

/**
 * Converts an EPUB to a KEPUB. The EPUB may be a Blob (e.g., a File), which is
 * read as needed when running in a worker, and read into memory first
 * otherwise.
 */
export async function convert(epub: Input, options: Options = {}): Promise<Uint8Array<ArrayBuffer>> {
	return call(await getModule(), "convert", epub, options);
}

/**
 * Converts an EPUB to a KEPUB, returning it as a Blob. This uses less memory
 * than {@link convert} since the output doesn't need to be copied into a new
 * buffer.
 */
export async function convertBlob(epub: Input, options: Options = {}): Promise<Blob> {
	const { blob, crc32 } = await callBlob(await getModule(), epub, options);
	return withCRC32(blob, crc32);
}

/** Transforms a single (X)HTML content document, returning it as UTF-8. */
export async function transformContent(html: string | Uint8Array, options: Options = {}): Promise<string> {
	const [input, o] = contentInput(html, options);
	return dec.decode(await call(await getModule(), "transform_content", input, o));
}
