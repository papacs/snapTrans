package com.papacs.snaptrans.capture

import java.util.concurrent.atomic.AtomicReference

/**
 * Each request is a distinct identity token. Register/cancel/clear may run on
 * the caller thread; armedPending/arm/claimArmed/disarm belong to the capture handler only.
 * Publishing a pending request does not make old queued images eligible for it.
 */
internal class CaptureRequestSlot<T : Any> {
    private val pendingRequest = AtomicReference<T?>(null)
    private var armedRequest: T? = null

    val pending: T? get() = pendingRequest.get()

    val armedPending: T? get() {
        val request = armedRequest ?: return null
        return if (isPending(request)) request else null
    }

    fun register(request: T): Boolean = pendingRequest.compareAndSet(null, request)

    fun isPending(request: T): Boolean = pendingRequest.get() === request

    fun cancel(request: T): Boolean = pendingRequest.compareAndSet(request, null)

    fun clear(): T? = pendingRequest.getAndSet(null)

    fun arm(request: T): Boolean {
        if (!isPending(request)) return false
        armedRequest = request
        return true
    }

    fun claimArmed(): T? {
        val request = armedRequest ?: return null
        armedRequest = null
        return if (pendingRequest.compareAndSet(request, null)) request else null
    }

    fun disarm() { armedRequest = null }

    fun disarm(request: T) {
        if (armedRequest === request) armedRequest = null
    }
}
