import { type Request, type Response, AlreadyKepubError, call } from "./kepubify";

const scope = self as unknown as {
	onmessage: ((e: MessageEvent<Request>) => void) | null;
	postMessage(m: Response, transfer?: Transferable[]): void;
};

scope.onmessage = async ({ data: { module, fn, input, options } }) => {
	try {
		const output = await call(module, fn, input, {
			...options,
			warn: options.warn ? (name, msg) => scope.postMessage({ warn: [name, msg] }) : undefined,
			progress: options.progress ? percent => scope.postMessage({ progress: percent }) : undefined,
			memory: options.memory ? bytes => scope.postMessage({ memory: bytes }) : undefined,
		});
		scope.postMessage({ output }, [output.buffer]);
	} catch (err) {
		scope.postMessage({
			error: err instanceof Error ? err.message : String(err),
			alreadyKepub: err instanceof AlreadyKepubError,
		});
	}
};
