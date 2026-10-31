import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { inflateSync } from 'node:zlib'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const publicDir = resolve(dirname(fileURLToPath(import.meta.url)), '..', 'public')
const manifest = JSON.parse(readFileSync(resolve(publicDir, 'manifest.webmanifest'), 'utf8'))
const icons = [
  { filename: 'icon-192.png', size: 192 },
  { filename: 'icon-512.png', size: 512 },
]
const signature = Buffer.from([137, 80, 78, 71, 13, 10, 26, 10])
const crcTable = new Uint32Array(256)

for (let n = 0; n < crcTable.length; n += 1) {
  let crc = n
  for (let bit = 0; bit < 8; bit += 1) {
    crc = (crc & 1) === 1 ? 0xedb88320 ^ (crc >>> 1) : crc >>> 1
  }
  crcTable[n] = crc >>> 0
}

function crc32(bytes) {
  let crc = 0xffffffff
  for (const byte of bytes) {
    crc = crcTable[(crc ^ byte) & 0xff] ^ (crc >>> 8)
  }
  return (crc ^ 0xffffffff) >>> 0
}

function validatePng(bytes, { filename, size }) {
  assert.ok(bytes.subarray(0, signature.length).equals(signature), `${filename} has an invalid PNG signature`)

  let offset = signature.length
  let header
  let sawEnd = false
  const imageData = []

  while (offset < bytes.length) {
    assert.ok(offset + 12 <= bytes.length, `${filename} ends inside a PNG chunk header`)
    const length = bytes.readUInt32BE(offset)
    const type = bytes.toString('ascii', offset + 4, offset + 8)
    const dataStart = offset + 8
    const dataEnd = dataStart + length
    const chunkEnd = dataEnd + 4
    assert.ok(chunkEnd <= bytes.length, `${filename} ends inside its ${type} chunk`)

    const data = bytes.subarray(dataStart, dataEnd)
    const expectedCrc = crc32(bytes.subarray(offset + 4, dataEnd))
    const actualCrc = bytes.readUInt32BE(dataEnd)
    assert.equal(actualCrc, expectedCrc, `${filename} has an invalid ${type} CRC`)

    if (type === 'IHDR') {
      assert.equal(offset, signature.length, `${filename} must start with IHDR`)
      assert.equal(length, 13, `${filename} has an invalid IHDR length`)
      header = {
        width: data.readUInt32BE(0),
        height: data.readUInt32BE(4),
        bitDepth: data[8],
        colorType: data[9],
        interlace: data[12],
      }
    } else if (type === 'IDAT') {
      imageData.push(data)
    } else if (type === 'IEND') {
      assert.equal(length, 0, `${filename} has a non-empty IEND chunk`)
      sawEnd = true
      offset = chunkEnd
      break
    }

    offset = chunkEnd
  }

  assert.ok(header, `${filename} has no IHDR chunk`)
  assert.equal(header.width, size, `${filename} width must be ${size}px`)
  assert.equal(header.height, size, `${filename} height must be ${size}px`)
  assert.equal(header.bitDepth, 8, `${filename} must use 8-bit channels`)
  assert.equal(header.colorType, 6, `${filename} must preserve transparent corners as RGBA`)
  assert.equal(header.interlace, 0, `${filename} must be non-interlaced`)
  assert.ok(imageData.length > 0, `${filename} has no image data`)
  assert.ok(sawEnd, `${filename} has no IEND chunk`)
  assert.equal(offset, bytes.length, `${filename} has trailing or truncated data`)

  const pixels = inflateSync(Buffer.concat(imageData))
  const rowLength = size * 4 + 1
  assert.equal(pixels.length, rowLength * size, `${filename} pixel data is incomplete`)
  for (let row = 0; row < size; row += 1) {
    assert.ok(pixels[row * rowLength] <= 4, `${filename} row ${row} has an invalid PNG filter`)
  }
  console.log(`${filename}: valid ${size}x${size} RGBA PNG`)
}

function validateIcon({ filename, size }) {
  const manifestEntry = manifest.icons.find(
    (entry) => entry.src === `/${filename}` && entry.sizes === `${size}x${size}`,
  )
  assert.ok(manifestEntry, `${filename} must be referenced at ${size}x${size} in manifest.webmanifest`)
  assert.equal(manifestEntry.type, 'image/png', `${filename} must be declared as PNG`)

  validatePng(readFileSync(resolve(publicDir, filename)), { filename, size })
}

for (const icon of icons) validateIcon(icon)

const sample = readFileSync(resolve(publicDir, 'icon-192.png'))
const corruptedSample = Buffer.from(sample)
const idatType = corruptedSample.indexOf(Buffer.from('IDAT'))
assert.ok(idatType >= 0, 'icon-192.png must contain an IDAT chunk')
corruptedSample[idatType + 4] ^= 0x01
assert.throws(
  () => validatePng(corruptedSample, { filename: 'corrupted fixture', size: 192 }),
  /invalid IDAT CRC/,
)
console.log('corrupted PNG fixture: rejected')
