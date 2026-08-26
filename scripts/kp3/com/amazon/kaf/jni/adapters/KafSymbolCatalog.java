package com.amazon.kaf.jni.adapters;

/**
 * Dump DigitalBook's native symbol-id/name mapping for an inclusive ID range.
 *
 * This reaches DigitalBook.nativeGetSymbolName via the public package adapter
 * method f(long). Unlike PropertyNameUtil, it includes non-property YJ shared
 * symbols (for example the yj.conversion.* symbols added after property 853)
 * and the book's local symbols after the shared-table boundary.
 */
public final class KafSymbolCatalog {
    public static void main(String[] args) throws Exception {
        if (args.length < 1 || args.length > 3) {
            System.err.println("usage: KafSymbolCatalog <book.kdf> [start-id] [end-id]");
            System.exit(2);
        }
        long start = args.length >= 2 ? Long.parseLong(args[1]) : 10;
        long end = args.length >= 3 ? Long.parseLong(args[2]) : 875;
        if (end < start) {
            throw new IllegalArgumentException("end-id must be >= start-id");
        }

        c.a();
        BookFactory factory = new BookFactory();
        factory.b();
        DigitalBook book = (DigitalBook) factory.a(args[0]);
        for (long id = start; id <= end; id++) {
            try {
                System.out.println(id + "\t" + String.valueOf(book.f(id)));
            } catch (Throwable t) {
                System.out.println(id + "\t<ERR:" + t.getClass().getSimpleName() + ">");
            }
        }
        System.out.flush();
        // Avoid broad JNI teardown; the other KAF probes use the same one-shot
        // process policy because some native ownership paths are fragile.
        System.exit(0);
    }
}
