package com.papacs.snaptrans.capture

import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test

class CaptureRequestSlotTest {
    @Test fun `pending request cannot consume images until its handler arms it`() {
        val slot = CaptureRequestSlot<Any>()
        val request = Any()
        assertTrue(slot.register(request))
        assertNull(slot.claimArmed())
        assertSame(request, slot.pending)
        assertTrue(slot.arm(request))
        assertSame(request, slot.claimArmed())
        assertNull(slot.pending)
        assertNull(slot.claimArmed())
    }

    @Test fun `queued old image cannot consume a new pending request`() {
        val slot = CaptureRequestSlot<Any>()
        val old = Any()
        val current = Any()
        assertTrue(slot.register(old))
        assertTrue(slot.arm(old))
        assertTrue(slot.cancel(old))
        assertTrue(slot.register(current))
        assertNull(slot.claimArmed())
        assertSame(current, slot.pending)
        assertTrue(slot.arm(current))
        assertSame(current, slot.claimArmed())
    }

    @Test fun `canceled setup cannot arm or displace a newer armed request`() {
        val slot = CaptureRequestSlot<Any>()
        val old = Any()
        val current = Any()
        assertTrue(slot.register(old))
        assertTrue(slot.cancel(old))
        assertTrue(slot.register(current))
        assertFalse(slot.arm(old))
        assertTrue(slot.arm(current))
        assertFalse(slot.arm(old))
        assertSame(current, slot.claimArmed())
    }

    @Test fun `clear removes only the pending owner and leaves a retry protected`() {
        val slot = CaptureRequestSlot<Any>()
        val old = Any()
        val current = Any()
        assertTrue(slot.register(old))
        assertTrue(slot.arm(old))
        assertSame(old, slot.clear())
        assertTrue(slot.register(current))
        assertNull(slot.claimArmed())
        assertSame(current, slot.pending)
    }

    @Test fun `resize can disarm and rearm the same pending request`() {
        val slot = CaptureRequestSlot<Any>()
        val request = Any()
        assertTrue(slot.register(request))
        assertTrue(slot.arm(request))
        slot.disarm()
        assertNull(slot.claimArmed())
        assertSame(request, slot.pending)
        assertTrue(slot.arm(request))
        assertSame(request, slot.claimArmed())
    }

    @Test fun `resize cannot arm a retry before its fresh capture setup`() {
        val slot = CaptureRequestSlot<Any>()
        val old = Any()
        val current = Any()
        assertTrue(slot.register(old))
        assertNull(slot.armedPending)
        assertTrue(slot.arm(old))
        assertSame(old, slot.armedPending)
        assertTrue(slot.cancel(old))
        assertTrue(slot.register(current))

        val previouslyArmed = slot.armedPending
        assertNull(previouslyArmed)
        slot.disarm()
        previouslyArmed?.let { slot.arm(it) }
        assertNull(slot.claimArmed())
        assertSame(current, slot.pending)

        assertTrue(slot.arm(current))
        assertSame(current, slot.armedPending)
        assertSame(current, slot.claimArmed())
        assertNull(slot.armedPending)
    }

    @Test fun `a stale cancellation does not cancel a newer request`() {
        val slot = CaptureRequestSlot<Any>()
        val old = Any()
        val current = Any()
        assertTrue(slot.register(old))
        assertTrue(slot.cancel(old))
        assertTrue(slot.register(current))
        assertTrue(slot.arm(current))
        assertFalse(slot.cancel(old))
        assertSame(current, slot.claimArmed())
    }

    @Test fun `stale disarm cleanup cannot clear a newer armed request`() {
        val slot = CaptureRequestSlot<Any>()
        val old = Any()
        val current = Any()
        assertTrue(slot.register(old))
        assertTrue(slot.arm(old))
        assertTrue(slot.cancel(old))
        assertTrue(slot.register(current))
        assertTrue(slot.arm(current))
        slot.disarm(old)
        assertSame(current, slot.claimArmed())
    }

    @Test fun `matching disarm preserves pending ownership for a fresh attach`() {
        val slot = CaptureRequestSlot<Any>()
        val request = Any()
        assertTrue(slot.register(request))
        assertTrue(slot.arm(request))
        slot.disarm(request)
        assertNull(slot.claimArmed())
        assertSame(request, slot.pending)
        assertTrue(slot.arm(request))
        assertSame(request, slot.claimArmed())
    }

    @Test fun `register rejects overlapping requests without replacing the owner`() {
        val slot = CaptureRequestSlot<Any>()
        val request = Any()
        assertTrue(slot.register(request))
        assertFalse(slot.register(Any()))
        assertTrue(slot.isPending(request))
        assertFalse(slot.isPending(Any()))
        assertTrue(slot.arm(request))
        assertSame(request, slot.claimArmed())
    }
}
