import com.amazon.yjreadersdk.*;
import com.amazon.yjreadersdk.interfaces.*;
import java.io.*;
import java.util.*;

public class KFXVoucherExtractor {
    private static final String ACSR_PATH = "/var/local/java/prefs/acsr";

    static String readAccountSecret(String acsrPath) {
        File acsrFile = new File(acsrPath);
        if (acsrFile.isFile()) {
            try {
                String acsr = new String(java.nio.file.Files.readAllBytes(
                    java.nio.file.Paths.get(acsrPath))).trim();
                if (!acsr.isEmpty()) {
                    return acsr;
                }
            } catch (IOException e) {
                System.err.println("WARNING: Could not read account secret at " + acsrPath + ": " + e.getMessage());
            }
        }

        System.err.println("WARNING: Account secret is missing or empty; continuing with device serial only.");
        System.err.println("This is expected on older Kindle firmware.");
        return "";
    }

    static class Arguments {
        final String serial;
        final String accountSecretOverride;
        final List<String> voucherPaths;

        Arguments(String serial, String accountSecretOverride, List<String> voucherPaths) {
            this.serial = serial;
            this.accountSecretOverride = accountSecretOverride;
            this.voucherPaths = voucherPaths;
        }
    }

    /**
     * Parses {@code <serial> [--acsr <secret>] [voucher ...]}.
     *
     * The override lets the Python driver pin one account secret per JVM run
     * when the device ACSR file holds several comma-separated secrets; the
     * SDK accepts a single ACCOUNT_SECRET lock parameter per run. Returns
     * null when the arguments are malformed.
     */
    static Arguments parseArguments(String[] args) {
        if (args.length == 0) {
            return null;
        }
        String serial = args[0];
        String override = null;
        List<String> vouchers = new ArrayList<>();
        int i = 1;
        while (i < args.length) {
            if ("--acsr".equals(args[i])) {
                if (i + 1 >= args.length) {
                    return null;
                }
                override = args[i + 1];
                i += 2;
            } else {
                vouchers.add(args[i]);
                i += 1;
            }
        }
        return new Arguments(serial, override, vouchers);
    }

    public static void main(String[] args) throws Exception {
        Arguments arguments = parseArguments(args);
        if (arguments == null) {
            System.err.println("Usage: KFXVoucherExtractor <serial> [--acsr <secret>] [voucher ...]");
            System.exit(1);
        }

        String acsr = arguments.accountSecretOverride != null
            ? arguments.accountSecretOverride
            : readAccountSecret(ACSR_PATH);
        String serial = arguments.serial;

        IBookSecurity sec = BookSecurity.getNativeInstance();
        sec.setAccountSecrets(acsr);
        Map<String, String> params = new HashMap<>();
        params.put("ACCOUNT_SECRET", acsr);
        params.put("CLIENT_ID", serial);
        sec.setLockParameters(params);
        System.out.println("Security initialized");

        List<File> vouchers = new ArrayList<>();
        for (String voucherPath : arguments.voucherPaths) {
            File f = new File(voucherPath);
            if (f.exists()) {
                System.out.println("Voucher: " + voucherPath);
                vouchers.add(f);
            }
        }
        sec.attachVouchers(vouchers);
        System.out.println("All vouchers attached");

        Thread.sleep(2000);
        sec.dispose();
        System.out.println("Done");
    }
}
