package com.amazon.yjreadersdk;

import com.amazon.yjreadersdk.interfaces.IBookSecurity;

/**
 * Minimal compile-time stub for the Kindle YJReader SDK entry point.
 * The real implementation is supplied by the Kindle and is not packaged.
 */
public final class BookSecurity {
    private BookSecurity() {
    }

    public static IBookSecurity getNativeInstance() {
        throw new UnsupportedOperationException("Kindle runtime implementation required");
    }
}
