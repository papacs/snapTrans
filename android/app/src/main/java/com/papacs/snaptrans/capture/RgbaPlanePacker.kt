package com.papacs.snaptrans.capture

import java.nio.ByteBuffer

/** Packs visible RGBA pixels without requiring padding after the final row. */
internal fun packRgbaPlane(
    source: ByteBuffer,
    width: Int,
    height: Int,
    pixelStride: Int,
    rowStride: Int,
): ByteBuffer {
    require(width > 0 && height > 0) { "Invalid screenshot dimensions" }
    val pixels = width.toLong() * height
    require(pixels <= Int.MAX_VALUE / 4) { "Screenshot is too large" }
    require(pixelStride >= 4) { "Invalid RGBA pixel stride" }
    val visibleRowBytes = (width - 1).toLong() * pixelStride + 4
    require(rowStride.toLong() >= visibleRowBytes) { "Invalid RGBA row stride" }
    val requiredBytes = (height - 1).toLong() * rowStride + visibleRowBytes
    require(requiredBytes <= source.remaining().toLong()) { "Screenshot pixel buffer is incomplete" }

    val packedSize = (pixels * 4).toInt()
    val rowBytes = width * 4
    if (pixelStride == 4 && rowStride == rowBytes) {
        return source.slice().apply { limit(packedSize) }
    }

    val packed = ByteArray(packedSize)
    val input = source.duplicate()
    val origin = source.position()
    for (row in 0 until height) {
        val rowOffset = origin + row * rowStride
        if (pixelStride == 4) {
            input.position(rowOffset)
            input.get(packed, row * rowBytes, rowBytes)
        } else {
            for (column in 0 until width) {
                input.position(rowOffset + column * pixelStride)
                input.get(packed, row * rowBytes + column * 4, 4)
            }
        }
    }
    return ByteBuffer.wrap(packed)
}
