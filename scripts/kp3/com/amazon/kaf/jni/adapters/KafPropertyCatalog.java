package com.amazon.kaf.jni.adapters;

import com.amazon.kaf.c.ab;
import com.amazon.kaf.util.PropertyNameUtil;
import java.util.*;

/** Dump the property-id/name table from Previewer's live native KAF runtime. */
public final class KafPropertyCatalog {
    public static void main(String[] args) throws Exception {
        c.a();
        List<ab> properties = PropertyNameUtil.a();
        properties.sort(Comparator.comparingLong(ab::d));
        for (ab property : properties) {
            System.out.println(property.d() + "\t" + property.a());
        }
        System.out.flush();
        System.exit(0);
    }
}
