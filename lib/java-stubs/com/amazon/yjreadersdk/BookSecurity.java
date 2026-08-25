package com.amazon.yjreadersdk;

import com.amazon.yjreadersdk.interfaces.IBookSecurity;
import java.io.File;
import java.util.List;
import java.util.Map;

/**
 * Test-only recording stand-in for the Kindle YJReader SDK entry point.
 *
 * The real implementation is supplied by the Kindle firmware
 * (/opt/amazon/ebook/lib/YJReader-impl.jar) and is never packaged or shipped.
 * This stub lets the bundled KFXVoucherExtractor run end to end in tests: it
 * records every SDK call on stdout as {@code REC:<method>:<details>} lines and
 * enforces the contract call order
 *
 *   setAccountSecrets → setLockParameters → attachVouchers → dispose
 *
 * Any other order (including a repeated or premature call) throws
 * IllegalStateException so contract violations fail loudly instead of
 * silently succeeding. Tests assert the recorded values; this class asserts
 * the sequence.
 */
public final class BookSecurity {
    private BookSecurity() {
    }

    public static IBookSecurity getNativeInstance() {
        return new RecordingBookSecurity();
    }

    static final class RecordingBookSecurity implements IBookSecurity {
        private int stage = 0;

        private void requireStage(int expected, String method) {
            if (stage != expected) {
                throw new IllegalStateException(
                        method + " called at stage " + stage + "; expected " + expected);
            }
            stage = expected + 1;
        }

        @Override
        public void setAccountSecrets(String accountSecrets) {
            requireStage(0, "setAccountSecrets");
            System.out.println("REC:setAccountSecrets:" + accountSecrets);
        }

        @Override
        public void setLockParameters(Map<String, String> parameters) {
            requireStage(1, "setLockParameters");
            StringBuilder recorded = new StringBuilder();
            for (Map.Entry<String, String> entry : parameters.entrySet()) {
                if (recorded.length() > 0) {
                    recorded.append(';');
                }
                recorded.append(entry.getKey()).append('=').append(entry.getValue());
            }
            System.out.println("REC:setLockParameters:" + recorded);
        }

        @Override
        public void attachVouchers(List<File> vouchers) {
            requireStage(2, "attachVouchers");
            for (File voucher : vouchers) {
                System.out.println("REC:attachVoucher:" + voucher.getPath());
            }
        }

        @Override
        public void dispose() {
            requireStage(3, "dispose");
            System.out.println("REC:dispose");
        }
    }
}
