package com.papacs.snaptrans.capture

import java.nio.ByteBuffer
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class RgbaPlanePackerTest {
    @Test fun `padded rows do not require padding beyond the final pixel`() {
        val bytes = ByteArray(28) { it.toByte() }
        val expected = bytes.copyOfRange(0, 12) + bytes.copyOfRange(16, 28)
        for (buffer in listOf(ByteBuffer.wrap(bytes), ByteBuffer.allocateDirect(bytes.size).apply { put(bytes); flip() })) {
            assertArrayEquals(expected, packRgbaPlane(buffer, 3, 2, 4, 16).remainingBytes())
            assertEquals(0, buffer.position())
            assertEquals(28, buffer.limit())
        }
    }

    @Test fun `packing preserves source position and limit`() {
        val bytes = ByteArray(40) { it.toByte() }
        val source = ByteBuffer.wrap(bytes).apply { position(8); limit(36) }
        val expected = bytes.copyOfRange(8, 20) + bytes.copyOfRange(24, 36)
        assertArrayEquals(expected, packRgbaPlane(source, 3, 2, 4, 16).remainingBytes())
        assertEquals(8, source.position())
        assertEquals(36, source.limit())
    }

    @Test fun `tight rows return a zero-copy slice`() {
        val bytes = ByteArray(20) { it.toByte() }
        val source = ByteBuffer.wrap(bytes).apply { position(4) }
        val packed = packRgbaPlane(source, 2, 2, 4, 8)
        assertEquals(16, packed.remaining())
        assertEquals(4, source.position())
        bytes[4] = 91
        assertEquals(91.toByte(), packed.get(0))
        assertArrayEquals(bytes.copyOfRange(4, 20), packed.remainingBytes())
    }

    @Test fun `pixel stride padding is skipped`() {
        val source = ByteArray(32) { it.toByte() }
        val expected = source.copyOfRange(0, 4) + source.copyOfRange(8, 12) +
            source.copyOfRange(20, 24) + source.copyOfRange(28, 32)
        assertArrayEquals(expected, packRgbaPlane(ByteBuffer.wrap(source), 2, 2, 8, 20).remainingBytes())
    }

    @Test fun `one pixel does not require its trailing stride`() {
        val source = byteArrayOf(1, 2, 3, 4)
        assertArrayEquals(source, packRgbaPlane(ByteBuffer.wrap(source), 1, 1, 8, 64).remainingBytes())
    }

    @Test fun `invalid or truncated layouts fail before allocation`() {
        assertThrows(IllegalArgumentException::class.java) { packRgbaPlane(ByteBuffer.allocate(27), 3, 2, 4, 16) }
        assertThrows(IllegalArgumentException::class.java) { packRgbaPlane(ByteBuffer.allocate(4), 0, 1, 4, 4) }
        assertThrows(IllegalArgumentException::class.java) { packRgbaPlane(ByteBuffer.allocate(4), 1, 1, 3, 4) }
        assertThrows(IllegalArgumentException::class.java) { packRgbaPlane(ByteBuffer.allocate(16), 3, 1, 4, 8) }
        assertThrows(IllegalArgumentException::class.java) { packRgbaPlane(ByteBuffer.allocate(0), Int.MAX_VALUE, 2, 4, Int.MAX_VALUE) }
    }

    private fun ByteBuffer.remainingBytes(): ByteArray = ByteArray(remaining()).also { get(it) }
}
