package com.amazon.ebook.booklet.reader.sdk.annotation.sync;
import java.util.Optional;
public interface AnnotationSyncClientProxy {
    Optional<SaveReadingProgressResponse> a(KSDKAnnotationsApiRequest request);
}
