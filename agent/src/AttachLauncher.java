import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;

/** Loads a Java agent into the local Kindle framework JVM. */
public final class AttachLauncher {
    private AttachLauncher() {}

    public static void main(String[] args) throws Exception {
        if (args.length < 2 || args.length > 3) {
            System.err.println("usage: AttachLauncher <pid> <agent.jar> [agent-args]");
            System.exit(2);
        }
        Class<?> vmClass = Class.forName("com.sun.tools.attach.VirtualMachine");
        Method attach = vmClass.getMethod("attach", String.class);
        Method loadAgent = vmClass.getMethod("loadAgent", String.class, String.class);
        Method detach = vmClass.getMethod("detach");
        Object vm = null;
        try {
            vm = attach.invoke(null, args[0]);
            loadAgent.invoke(vm, args[1], args.length == 3 ? args[2] : "");
        } catch (InvocationTargetException error) {
            Throwable cause = error.getCause();
            if (cause instanceof Exception) throw (Exception) cause;
            throw error;
        } finally {
            if (vm != null) detach.invoke(vm);
        }
    }
}
