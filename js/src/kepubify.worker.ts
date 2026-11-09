import { type Request, type Response, AlreadyKepubError, call, callBlob } from "./kepubify";

const scope = self as unknown as {
	onmessage: ((e: MessageEvent<Request>) => void) | null;
	postMessage(m: Response, transfer?: Transferable[]): void;
};

scope.onmessage = async ({ data: { module, fn, input, blob, options } }) => {
	try {
		const o = {
			...options,
			warn: options.warn ? (name: string, msg: string) => scope.postMessage({ warn: [name, msg] }) : undefined,
			progress: options.progress ? (percent: number) => scope.postMessage({ progress: percent }) : undefined,
			memory: options.memory ? (bytes: number) => scope.postMessage({ memory: bytes }) : undefined,
		};
		if (blob) {
			scope.postMessage(await callBlob(module, input, o));
		} else {
			const output = await call(module, fn, input, o);
			scope.postMessage({ output }, [output.buffer]);
		}
	} catch (err) {
		scope.postMessage({
			error: err instanceof Error ? err.message : String(err),
			alreadyKepub: err instanceof AlreadyKepubError,
		});
	}
};
