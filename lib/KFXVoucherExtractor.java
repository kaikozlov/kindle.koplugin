import com.amazon.yjreadersdk.*;
import com.amazon.yjreadersdk.interfaces.*;
import java.io.*;
import java.util.*;

public class KFXVoucherExtractor {
    public static void main(String[] args) throws Exception {
        String acsrPath = "/var/local/java/prefs/acsr";
        if (!new File(acsrPath).exists()) {
            System.err.println("ERROR: Account secret not found at " + acsrPath);
            System.err.println("Your Kindle must be registered to an Amazon account to access DRM-protected books.");
            System.err.println("Register your device, run Refresh Book Access, then you can deregister again.");
            System.exit(1);
        }
        String acsr = new String(java.nio.file.Files.readAllBytes(
            java.nio.file.Paths.get(acsrPath))).trim();
        if (args.length == 0) {
            System.err.println("Usage: KFXVoucherExtractor <serial> [voucher ...]");
            System.exit(1);
        }
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
