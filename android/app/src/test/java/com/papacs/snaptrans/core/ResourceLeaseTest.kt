package com.papacs.snaptrans.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test
import kotlinx.coroutines.CancellableContinuation
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.launch
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest

@OptIn(ExperimentalCoroutinesApi::class)
class ResourceLeaseTest {
    @Test fun `a captured bitmap is released when cancellation wins before or after resume`() = runTest {
        for (cancelBeforeResume in listOf(false, true)) {
            var released = 0
            val pending = CompletableDeferred<CancellableContinuation<ResourceLease<String>>>()
            val job = launch {
                val frame = suspendCancellableCoroutine<ResourceLease<String>> { pending.complete(it) }
                frame.use { error("cancelled capture must not consume pixels") }
            }
            runCurrent()
            if (cancelBeforeResume) job.cancel()
            pending.await().resumeWithResource(ResourceLease("pixels") { released++ })
            job.cancel()
            runCurrent()
            assertEquals(1, released)
        }
    }

    @Test fun `cancelling a bitmap owner does not free pixels still read by OCR`() {
        var released = 0
        val screen = ResourceLease("screen pixels") { released++ }
        val sdk = screen.retain()
        screen.close()
        assertEquals(0, released)
        assertEquals("screen pixels", sdk.value)
        sdk.close()
        assertEquals(1, released)
    }

    @Test fun `all handles close idempotently in any completion order`() {
        var released = 0
        val screen = ResourceLease("screen pixels") { released++ }
        val optimized = screen.retain()
        val sdk = optimized.retain()
        sdk.close()
        sdk.close()
        screen.close()
        assertEquals(0, released)
        optimized.close()
        optimized.close()
        assertEquals(1, released)
    }

    @Test fun `closed handles cannot start another asynchronous reader`() {
        val screen = ResourceLease("screen pixels") { }
        screen.close()
        assertThrows(IllegalStateException::class.java) { screen.retain() }
        assertThrows(IllegalStateException::class.java) { screen.value }
    }
}
