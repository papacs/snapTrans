package com.papacs.snaptrans.core

import kotlinx.coroutines.CancellableContinuation

/** Each asynchronous reader retains its own handle; only the final close frees pixels. */
class ResourceLease<T> private constructor(private val shared: Shared<T>) : AutoCloseable {
    private class Shared<T>(val value: T, val release: (T) -> Unit) {
        var references = 1
    }

    constructor(value: T, release: (T) -> Unit) : this(Shared(value, release))
    private var closed = false

    val value: T get() = synchronized(shared) {
        check(!closed) { "Resource lease is closed" }
        shared.value
    }

    fun retain(): ResourceLease<T> = synchronized(shared) {
        check(!closed) { "Resource lease is closed" }
        shared.references++
        ResourceLease(shared)
    }

    override fun close() = synchronized(shared) {
        if (!closed) {
            closed = true
            if (--shared.references == 0) shared.release(shared.value)
        }
    }
}

internal fun <T> CancellableContinuation<ResourceLease<T>>.resumeWithResource(resource: ResourceLease<T>) {
    resume(resource) { _, discarded, _ -> discarded.close() }
}
