import { brotliCompressSync } from "zlib"
import { Buffer } from "buffer"
import { env } from "process"
import { fileURLToPath } from "url"

import copy from "rollup-plugin-copy"
import gzip from "rollup-plugin-gzip"
import resolve from "@rollup/plugin-node-resolve"
import typescript from "rollup-plugin-typescript2"
import workerLoader from "rollup-plugin-web-worker-loader"
import html from '@web/rollup-plugin-html'
import { importMetaAssets } from "@web/rollup-plugin-import-meta-assets"
import { terser } from "rollup-plugin-terser"

const dev = env.NODE_ENV != "production"

const kepubify = {
    input: {
        kepubify: "src/kepubify/index.ts",
    },
    output: {
        format: "umd",
        name: "kepubify",
        sourcemap: true,
        dir: "dist",
        entryFileNames: "[name].js",
        assetFileNames: "[name]-[hash][extname]",
        banner: "/*! kepubify */",
    },
    plugins: [
        resolve({
            modulesOnly: true,
            preferBuiltins: true, // because of wasm_exec
        }),
        workerLoader({
            extensions: [".ts"],
        }),
        typescript({
            clean: true,
        }),
        importMetaAssets(),
        terser({
            compress: !dev,
            format: {
                comments: dev,
            },
            keep_classnames: true,
            keep_fnames: true,
        }),
        gzip({
            filter: dev
                ? /(?!)/
                : /\.wasm$/,
        }),
        gzip({
            filter: dev
                ? /(?!)/
                : /\.wasm$/,
            fileName: ".br",
            customCompression: x => brotliCompressSync(Buffer.from(x)),
        }),
    ],
}

const kepubify_try = {
    input: {
        kepubify_try: "src/kepubify-try/index.ts",
    },
    external: [
        "../kepubify",
    ],
    output: {
        globals: {
            [fileURLToPath(new URL("../kepubify", new URL("src/kepubify-try/index.ts", import.meta.url)))]: kepubify.output.name,
        },
        format: "iife",
        name: "kepubify_try",
        sourcemap: true,
        dir: "dist",
        entryFileNames: "[name].js",
    },
    plugins: [
        resolve({
            modulesOnly: true,
        }),
        typescript({
            clean: true,
        }),
        terser({
            compress: !dev,
            format: {
                comments: dev,
            },
            keep_classnames: true,
            keep_fnames: true,
        }),
    ],
}

const website = {
    input: "**/*.html",
    output: {
        dir: "dist/public",
        entryFileNames: "[name]-[hash].js",
        assetFileNames: "assets/[name]-[hash][extname]",
    },
    plugins: [
        html({
            flattenOutput: false,
            minify: false,
            extractAssets: true,
            rootDir: fileURLToPath(new URL("./public", import.meta.url)),
        }),
        copy({
            targets: [
                { src: "dist/*.wasm*", dest: "dist/public/assets" },
                { src: "dist/*.js.map*", dest: "dist/public/assets" },
            ],
        }),
    ],
}

export default [kepubify, kepubify_try].concat(dev ? [] : [website])
