package com.amazon.ebook.booklet.reader.sdk;
import com.amazon.ebook.booklet.reader.sdk.content.Book;
import com.amazon.ebook.booklet.reader.sdk.content.PositionFactory;
public interface ReaderContentSDK {
    Book dt(String path);
    PositionFactory E(Book book);
}
