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

    public static void main(String[] args) throws Exception {
        if (args.length == 0) {
            System.err.println("Usage: KFXVoucherExtractor <serial> [voucher ...]");
            System.exit(1);
        }

        String acsr = readAccountSecret(ACSR_PATH);
        String serial = args[0];
        
        IBookSecurity sec = BookSecurity.getNativeInstance();
        sec.setAccountSecrets(acsr);
        Map<String, String> params = new HashMap<>();
        params.put("ACCOUNT_SECRET", acsr);
        params.put("CLIENT_ID", serial);
        sec.setLockParameters(params);
        System.out.println("Security initialized");
        
        List<File> vouchers = new ArrayList<>();
        for (int i = 1; i < args.length; i++) {
            File f = new File(args[i]);
            if (f.exists()) {
                System.out.println("Voucher: " + args[i]);
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
