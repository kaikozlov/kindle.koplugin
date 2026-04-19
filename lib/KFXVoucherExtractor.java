import com.amazon.yjreadersdk.*;
import com.amazon.yjreadersdk.interfaces.*;
import java.io.*;
import java.util.*;

public class KFXVoucherExtractor {
    public static void main(String[] args) throws Exception {
        String acsr = new String(java.nio.file.Files.readAllBytes(
            java.nio.file.Paths.get("/var/local/java/prefs/acsr"))).trim();
        String serial = args.length > 0 ? args[0] : "REDACTED";
        
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
