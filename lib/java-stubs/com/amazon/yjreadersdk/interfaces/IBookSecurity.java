package com.amazon.yjreadersdk.interfaces;

import java.io.File;
import java.util.List;
import java.util.Map;

/**
 * Minimal compile-time stub for the Kindle YJReader SDK interface used by
 * KFXVoucherExtractor. The real implementation is supplied by the Kindle.
 */
public interface IBookSecurity {
    void setAccountSecrets(String accountSecrets);
    void setLockParameters(Map<String, String> parameters);
    void attachVouchers(List<File> vouchers);
    void dispose();
}
