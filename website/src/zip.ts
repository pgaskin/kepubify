const crcTable = new Uint32Array(256).map((_, n) => {
    let c = n
    for (let k = 0; k < 8; k++) {
        c = c & 1 ? 0xEDB88320 ^ (c >>> 1) : c >>> 1
    }
    return c
})

export function crc32(b: Uint8Array): number {
    let c = 0xFFFFFFFF
    for (let i = 0; i < b.length; i++) {
        c = crcTable[(c ^ b[i]) & 0xFF] ^ (c >>> 8)
    }
    return (c ^ 0xFFFFFFFF) >>> 0
}

export interface ZipEntry {
    name: string
    data: Blob
    crc32: number
}

// zip returns an uncompressed ZIP archive of the entries, modified at date.
// ZIP64 isn't supported.
export function zip(entries: ZipEntry[], date = new Date()): Blob {
    if (entries.length > 0xFFFF) {
        throw new Error("too many files for zip")
    }
    const enc = new TextEncoder()
    const time = date.getHours() << 11 | date.getMinutes() << 5 | date.getSeconds() >> 1
    const day = Math.max(date.getFullYear() - 1980, 0) << 9 | (date.getMonth() + 1) << 5 | date.getDate()
    const parts: BlobPart[] = []
    const central: Uint8Array<ArrayBuffer>[] = []
    let offset = 0
    for (const e of entries) {
        const name = enc.encode(e.name)
        const header = (sig: number, n: number, central: boolean) => {
            const b = new Uint8Array(n + name.length), v = new DataView(b.buffer)
            let p = 0
            const u16 = (x: number) => (v.setUint16(p, x, true), p += 2)
            const u32 = (x: number) => (v.setUint32(p, x, true), p += 4)
            u32(sig)
            if (central) u16(20) // version made by
            u16(20) // version needed
            u16(0x0800) // utf-8 names
            u16(0) // stored
            u16(time), u16(day)
            u32(e.crc32), u32(e.data.size), u32(e.data.size)
            u16(name.length), u16(0) // extra
            if (central) {
                u16(0) // comment
                u16(0), u16(0), u32(0) // disk, internal and external attributes
                u32(offset)
            }
            b.set(name, p)
            return b
        }
        const local = header(0x04034b50, 30, false)
        central.push(header(0x02014b50, 46, true))
        parts.push(local, e.data)
        offset += local.length + e.data.size
        if (offset > 0xFFFFFFFF) {
            throw new Error("files too large for zip")
        }
    }
    const size = central.reduce((n, b) => n + b.length, 0)
    const end = new Uint8Array(22), v = new DataView(end.buffer)
    v.setUint32(0, 0x06054b50, true)
    v.setUint16(8, entries.length, true)
    v.setUint16(10, entries.length, true)
    v.setUint32(12, size, true)
    v.setUint32(16, offset, true)
    return new Blob([...parts, ...central, end], { type: "application/zip" })
}
