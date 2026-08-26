package com.amazon.kaf.jni.adapters;

import com.amazon.kaf.c.*;
import java.io.FileDescriptor;
import java.io.FileOutputStream;
import java.io.PrintStream;
import java.util.*;

/**
 * Read-only probe for the native BookPositionInfo implementation.
 *
 * Exercises position-id <-> location conversion, Position/Offset objects,
 * KFXID string <-> EID conversion, and section lookup for positions.
 * Everything is printed incrementally with explicit flushing because some
 * native KAF ownership paths are unsafe; this is intentionally a one-shot
 * subprocess (same policy as KafSemanticProbe).
 */
public final class KafPositionProbe {
    private static final PrintStream OUT =
            new PrintStream(new FileOutputStream(FileDescriptor.out), true);

    private static void stage(String name) {
        OUT.println("## " + name);
    }

    private static String quote(String s) {
        if (s == null) return "null";
        return "\"" + s.replace("\\", "\\\\").replace("\n", "\\n").replace("\"", "\\\"") + "\"";
    }

    private static void describeOffset(Position pos) {
        try {
            T offset = pos.b();
            if (offset == null) {
                OUT.println("  offset=null");
                return;
            }
            Offset real = (Offset) offset;
            Object type;
            Object point;
            long value;
            try { type = real.a(); } catch (Throwable t) { type = "<ERR " + t.getClass().getSimpleName() + ">"; }
            try { point = real.c(); } catch (Throwable t) { point = "<ERR " + t.getClass().getSimpleName() + ">"; }
            try { value = real.b(); } catch (Throwable t) { value = -1; }
            OUT.println("  offset type=" + type + " value=" + value + " point=" + point);
        } catch (Throwable t) {
            OUT.println("  offset ERR " + t);
        }
    }

    public static void main(String[] args) throws Exception {
        if (args.length < 1) {
            System.err.println("usage: KafPositionProbe <book.kdf> [--unsafe-anchors] [location-map-out]");
            System.exit(2);
        }
        boolean unsafeAnchors = false;
        for (String arg : args) {
            if ("--unsafe-anchors".equals(arg)) unsafeAnchors = true;
        }

        c.a();
        BookFactory factory = new BookFactory();
        factory.b();
        DigitalBook book = (DigitalBook) factory.a(args[0]);
        BookContent content = (BookContent) book.d();

        o bpi = book.h();
        OUT.println("book=" + quote(args[0]));

        // -- Stage 1: global map sizes --------------------------------------
        stage("max");
        long maxPositionId = bpi.b();
        long maxLocation = bpi.a();
        OUT.println("maxPositionId=" + maxPositionId);
        OUT.println("maxLocation=" + maxLocation);

        // -- Stage 2: every position id -> Position (offset/eid semantics) --------
        // NOTE: Position.a() / Position_getNativePositionId returns the position
        // object's own ID field (element/EID-like), NOT the global position id.
        // Global PID conversion is BookPositionInfo_getNativePositionId (b(Y)),
        // native vtable +0x20 (convertToPositionID); the inverse used here,
        // getNativePositionforID (b(long)), is vtable +0x18 (convertToPosition).
        stage("positions");
        List<Long> validPids = new ArrayList<>();
        for (long pid = 0; pid <= Math.min(maxPositionId, 200000); pid++) {
            Y handle = null;
            try {
                handle = bpi.b(pid);
            } catch (Throwable t) {
                OUT.println("  pid=" + pid + " positionforID ERR " + t.getClass().getSimpleName());
                break;
            }
            if (handle == null) continue;
            validPids.add(pid);
            Position pos = (Position) handle;
            long eid;
            try { eid = pos.a(); } catch (Throwable t) { eid = -2; }
            long globalPid;
            try { globalPid = bpi.b(pos); } catch (Throwable t) { globalPid = -2; }
            OUT.println("pid=" + pid + " eid=" + eid + " globalPid=" + globalPid);
            describeOffset(pos);
        }
        OUT.println("valid_pids=" + validPids.size());

        // -- Stage 3: position <-> location ----------------------------------
        stage("location-roundtrip");
        for (long pid : validPids) {
            try {
                Y handle = bpi.b(pid);
                if (handle == null) continue;
                long location = bpi.a(handle);
                OUT.print("pid=" + pid + " location=" + location);
                Y back = bpi.a(location);
                if (back != null) {
                    Position p2 = (Position) back;
                    long pid2;
                    try { pid2 = bpi.b(p2); } catch (Throwable t) { pid2 = -2; }
                    OUT.print(" back.globalPid=" + pid2);
                } else {
                    OUT.print(" back=null");
                }
                OUT.println();
            } catch (Throwable t) {
                OUT.println("pid=" + pid + " location ERR " + t.getClass().getSimpleName() + " " + t.getMessage());
            }
        }

        // -- Stage 4: location -> Position (object eid+offset) and global PID -------
        stage("locations");
        for (long loc = 1; loc <= maxLocation; loc++) {
            try {
                Y handle = bpi.a(loc);
                if (handle == null) {
                    OUT.println("loc=" + loc + " position=null");
                } else {
                    Position pos = (Position) handle;
                    long eid;
                    try { eid = pos.a(); } catch (Throwable t) { eid = -2; }
                    long globalPid;
                    try { globalPid = bpi.b(pos); } catch (Throwable t) { globalPid = -2; }
                    OUT.println("loc=" + loc + " eid=" + eid + " globalPid=" + globalPid);
                    describeOffset(pos);
                }
            } catch (Throwable t) {
                OUT.println("loc=" + loc + " ERR " + t.getClass().getSimpleName());
            }
        }

        // -- Stage 5: collect eids from the graph and map KFXID <-> EID ------
        stage("kfxid-eid");
        Set<Long> eids = new TreeSet<>();
        try {
            for (String name : content.a(aB.o.Storyline)) {
                Storyline storyline = (Storyline) content.d(book.a(name));
                collectEids(storyline.g(), eids, 0);
            }
        } catch (Throwable t) {
            OUT.println("eid collection ERR " + t);
        }
        OUT.println("graph_eids=" + eids.size());
        for (long eid : eids) {
            try {
                String kfxid = bpi.e(eid);
                long back = kfxid == null ? -1 : bpi.c(kfxid);
                String name;
                try { name = book.f(eid); } catch (Throwable t) { name = "<ERR>"; }
                OUT.println("eid=" + eid + " name=" + quote(name) + " kfxid=" + quote(kfxid) + " back=" + back);
            } catch (Throwable t) {
                OUT.println("eid=" + eid + " kfxid ERR " + t.getClass().getSimpleName());
            }
        }

        // -- Stage 6: section lookup for positions ----------------------------
        stage("sections");
        for (long pid : validPids) {
            try {
                Y handle = bpi.b(pid);
                if (handle == null) continue;
                long sectionId = bpi.c(handle);
                String sectionName = sectionId == 0 ? null : book.f(sectionId);
                OUT.println("pid=" + pid + " sectionId=" + sectionId + " section=" + quote(sectionName));
            } catch (Throwable t) {
                OUT.println("pid=" + pid + " section ERR " + t.getClass().getSimpleName());
            }
        }

        // -- Stage 7: anchor lookup by eid --------------------------------------
        // UNSAFE: even an existence-only BookPositionInfo.getNativeAnchor call
        // (bpi.c(eid)) aborts the JVM after the preceding stages on these
        // fixtures. Gated behind an explicit --unsafe-anchors flag and executed
        // LAST; anchor->position evidence must come from fragment data
        // (anchors carry $511 target / $143 offset), not this call.
        if (unsafeAnchors) {
            stage("anchors");
            for (long eid : eids) {
                try {
                    h anchor = bpi.c(eid);
                    OUT.println("eid=" + eid + " anchor=" + (anchor == null ? "null" : anchor.getClass().getSimpleName()));
                } catch (Throwable t) {
                    OUT.println("eid=" + eid + " anchor ERR " + t.getClass().getSimpleName());
                }
            }
        }

        // -- Stage 8: serialize the native location map ------------------------
        if (args.length >= 2) {
            stage("serialize-location-map");
            try {
                bpi.b(args[1]);
                OUT.println("serialized to " + quote(args[1]));
            } catch (Throwable t) {
                OUT.println("serialize ERR " + t);
            }
        }

        OUT.flush();
        System.exit(0);
    }

    private static void collectEids(List<s> items, Set<Long> eids, int depth) {
        if (items == null || depth > 8) return;
        for (s item : items) {
            if (item == null) continue;
            try {
                Container container = (Container) item;
                eids.add(container.i());
                collectEids(container.m(), eids, depth + 1);
            } catch (Throwable ignored) {
            }
        }
    }
}
