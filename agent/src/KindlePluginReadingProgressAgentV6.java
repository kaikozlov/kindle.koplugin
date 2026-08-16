import com.amazon.ebook.booklet.reader.sdk.ReaderContentSDK;
import com.amazon.ebook.booklet.reader.sdk.ReaderSDK;
import com.amazon.ebook.booklet.reader.sdk.annotation.sync.AnnotationSyncClientProxy;
import com.amazon.ebook.booklet.reader.sdk.annotation.sync.KSDKBookDataWrapper;
import com.amazon.ebook.booklet.reader.sdk.annotation.sync.SaveReadingProgressRequest;
import com.amazon.ebook.booklet.reader.sdk.annotation.sync.SaveReadingProgressResponse;
import com.amazon.ebook.booklet.reader.sdk.content.Book;
import com.amazon.ebook.booklet.reader.sdk.content.LastPageRead;
import com.amazon.ebook.booklet.reader.sdk.content.LprSidecarAdapter;
import com.amazon.ebook.booklet.reader.sdk.content.Position;
import com.amazon.ebook.booklet.reader.impl.todo.ContentCatalogLprUtils;
import com.amazon.kindle.content.catalog.CatalogService;
import com.amazon.kindle.content.catalog.MutableItem;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileWriter;
import java.io.PrintWriter;
import java.lang.instrument.Instrumentation;
import java.nio.charset.StandardCharsets;
import java.util.Date;
import java.util.Optional;
import java.util.Properties;

/** Persists an exact KOReader location using Kindle's sidecar and ReaderSDK APIs. */
public final class KindlePluginReadingProgressAgentV6 {
    private KindlePluginReadingProgressAgentV6() {}

    public static void agentmain(String payloadPath, Instrumentation ignored) {
        PrintWriter out = null;
        Book book = null;
        String stage = "validate_payload";
        try {
            if (payloadPath == null || !payloadPath.matches("^/tmp/kindle-progress-[0-9]+\\.properties$"))
                throw new IllegalArgumentException("invalid payload path");
            Properties payload = new Properties();
            File payloadFile = new File(payloadPath);
            try (FileInputStream input = new FileInputStream(payloadFile)) { payload.load(input); }
            if (!payloadFile.delete()) throw new IllegalStateException("cannot remove consumed payload");
            if (!"1".equals(payload.getProperty("version"))) throw new IllegalArgumentException("unsupported payload");
            String requestId = requireDigits(payload, "request_id");
            if (!payloadPath.equals("/tmp/kindle-progress-" + requestId + ".properties"))
                throw new IllegalArgumentException("request ID mismatch");
            String asin = requireAsin(payload.getProperty("asin"));
            String nativePath = decodeHex(payload.getProperty("native_path_hex"));
            String operation = payload.getProperty("operation", "save");
            if (!"save".equals(operation) && !"read".equals(operation))
                throw new IllegalArgumentException("invalid operation");
            out = new PrintWriter(new FileWriter("/tmp/kindle-progress-result-" + requestId + ".log", false));
            out.println("request_id=" + requestId);
            out.println("asin=" + asin);

            stage = "resolve_reader_sdk";
            Class<?> framework = Class.forName("com.amazon.kindle.restricted.runtime.Framework");
            ReaderSDK sdk = (ReaderSDK) framework.getMethod("getService", Class.class)
                .invoke(null, ReaderSDK.class);
            ReaderContentSDK content = sdk.jE();

            stage = "open_book";
            book = content.dt(nativePath);
            if ("read".equals(operation)) {
                writeCurrentPosition(out, book);
                return;
            }

            String longPosition = requireLongPosition(payload.getProperty("long_position"));
            int expectedShort = Integer.parseInt(requireDigits(payload, "short_position"));
            stage = "translate_position";
            Position position = content.E(book).a(longPosition, book);
            int actualShort = position.nR();
            if (actualShort != expectedShort) throw new IllegalStateException("short position mismatch");

            // Book.Ud() returns an LPR through LprSidecarAdapter. Mutating that
            // detached object is not enough: it must be put back into the
            // adapter before Book.Ue() serializes the frequent-update sidecar.
            stage = "save_local_lpr";
            LprSidecarAdapter adapter = new LprSidecarAdapter(book, content);
            LastPageRead lpr = adapter.cA(false);
            if (lpr == null) lpr = new LastPageRead(content);
            lpr.a(position, new Date());
            if (!adapter.a(lpr)) throw new IllegalStateException("local LPR was not staged");
            book.Ue();

            // Prove durability through a fresh Book object before reporting
            // success or updating shelf/cloud metadata.
            book.close();
            book = null;
            stage = "verify_local_lpr";
            book = content.dt(nativePath);
            LastPageRead persisted = book.Ud();
            Position persistedPosition = persisted == null ? null : persisted.nz();
            if (persistedPosition == null
                    || persistedPosition.nR() != actualShort
                    || !position.nX().equals(persistedPosition.nX()))
                throw new IllegalStateException("local LPR durability check failed");
            out.println("local_progress_saved=true");
            out.println("local_progress_verified=true");
            out.flush();

            stage = "save_native_progress";
            Optional<KSDKBookDataWrapper> wrapped = KSDKBookDataWrapper.a(book.jg());
            if (!wrapped.isPresent()) throw new IllegalStateException("book data unavailable");
            AnnotationSyncClientProxy proxy = sdk.yl();
            SaveReadingProgressRequest request = new SaveReadingProgressRequest(
                wrapped.get(), actualShort, position.nX(), 30000L);
            Optional<SaveReadingProgressResponse> response = proxy.a(request);
            boolean accepted = response.isPresent() && response.get().cuY;
            double renderedPercent = nativePercent(persistedPosition);
            boolean catalogSaved = false;
            if (accepted) {
                stage = "refresh_reader_progress";
                sdk.xQ();
                stage = "save_catalog_progress";
                CatalogService catalog = (CatalogService) sdk.getService(CatalogService.class);
                MutableItem item = ContentCatalogLprUtils.a(catalog, nativePath);
                if (item == null) throw new IllegalStateException("native catalog item unavailable");
                item.setProperty("percentFinished", Float.valueOf((float) renderedPercent));
                catalogSaved = ContentCatalogLprUtils.a(sdk, catalog, item);
            }
            out.println("saved_short=" + actualShort);
            out.println("long_position=" + position.nX());
            out.println("native_percent=" + renderedPercent);
            out.println("native_progress_accepted=" + accepted);
            out.println("catalog_progress_saved=" + catalogSaved);
            out.println("success=" + (accepted && catalogSaved));
        } catch (Throwable error) {
            if (out != null) {
                out.println("failed_stage=" + stage);
                out.println("error_class=" + root(error).getClass().getName());
                out.println("success=false");
            }
        } finally {
            if (book != null) try { book.close(); } catch (Throwable ignoredClose) {}
            if (out != null) out.close();
        }
    }

    private static void writeCurrentPosition(PrintWriter out, Book book) {
        LastPageRead current = book.Ud();
        Position currentPosition = current == null ? null : current.nz();
        if (currentPosition == null) throw new IllegalStateException("native position unavailable");
        out.println("saved_short=" + currentPosition.nR());
        out.println("long_position=" + currentPosition.nX());
        out.println("native_percent=" + nativePercent(currentPosition));
        out.println("success=true");
    }

    private static double nativePercent(Position position) {
        // ReaderSyncDataSaver uses this rendered-progress fraction for the
        // content catalog's percentFinished property. Position IDs are not a
        // linear percentage scale and must not be divided by Book.jc().
        double percent = position.UG() * 100.0;
        if (!Double.isFinite(percent))
            throw new IllegalStateException("native rendered percentage unavailable");
        return Math.max(0.0, Math.min(100.0, percent));
    }

    private static String requireDigits(Properties values, String key) {
        String value = values.getProperty(key, "");
        if (!value.matches("^[0-9]+$")) throw new IllegalArgumentException("invalid " + key);
        return value;
    }
    private static String requireAsin(String value) {
        if (value == null || !value.matches("^B[A-Z0-9]{9}$")) throw new IllegalArgumentException("invalid ASIN");
        return value;
    }
    private static String requireLongPosition(String value) {
        if (value == null || !value.matches("^[A-Za-z0-9+/]{12}$")) throw new IllegalArgumentException("invalid long position");
        return value;
    }
    private static String decodeHex(String value) {
        if (value == null || (value.length() & 1) != 0 || !value.matches("^[0-9A-Fa-f]+$"))
            throw new IllegalArgumentException("invalid native path");
        byte[] bytes = new byte[value.length() / 2];
        for (int i = 0; i < bytes.length; i++)
            bytes[i] = (byte) Integer.parseInt(value.substring(i * 2, i * 2 + 2), 16);
        String result = new String(bytes, StandardCharsets.UTF_8);
        if (!result.startsWith("/mnt/us/documents/") || !result.endsWith(".kfx"))
            throw new IllegalArgumentException("unsafe native path");
        return result;
    }
    private static Throwable root(Throwable error) {
        Throwable current = error;
        while (current.getCause() != null) current = current.getCause();
        return current;
    }
}
