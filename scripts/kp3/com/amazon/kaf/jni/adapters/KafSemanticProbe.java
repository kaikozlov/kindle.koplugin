package com.amazon.kaf.jni.adapters;

import com.amazon.kaf.c.*;
import com.amazon.kaf.util.PropertyNameUtil;
import java.util.*;

/**
 * Read-only semantic probe for Kindle Previewer's bundled native KAF library.
 *
 * This deliberately lives in com.amazon.kaf.jni.adapters because several useful
 * JNI adapter constructors are package-private. Keep the traversal conservative:
 * some JNI getters have native lifetime/ownership assumptions and exploratory
 * calls outside this subset have crashed the bundled JVM.
 */
public final class KafSemanticProbe {
    private static String propertyName(long id) {
        try {
            return PropertyNameUtil.a((int) id).a();
        } catch (Throwable t) {
            return "$" + id;
        }
    }

    private static String quote(String s) {
        if (s == null) return "null";
        return "\"" + s.replace("\\", "\\\\").replace("\n", "\\n").replace("\"", "\\\"") + "\"";
    }

    private static String value(DigitalBook book, ac value, int depth) {
        if (value == null) return "null";
        if (depth > 4) return "<depth>";
        try {
            aB.P type = value.a();
            switch (type) {
                case kUnset:
                    return "unset";
                case kBool:
                    return Boolean.toString(value.b());
                case kString:
                case kExternString:
                case kParameterString:
                    return quote(value.a(book));
                case kDocSymbol:
                case kParameter:
                case kParameterSymbol: {
                    long id = value.e();
                    return "sym(" + id + ":" + quote(book.f(id)) + ")";
                }
                case kElemType: {
                    int id = value.i();
                    return "elem(" + id + ":" + propertyName(id) + ")";
                }
                case kFloat:
                case kFloatPoint:
                case kFloatPercent:
                case kFloatEm:
                case kFloatEx:
                case kFloatLH:
                case kFloatGridLine:
                case kFloatRem:
                case kFloatCh:
                case kFloatVW:
                case kFloatVH:
                case kFloatVMin:
                case kFloatVMax:
                    return Float.toString(value.f());
                case kInt:
                case kIntPoint:
                case kIntPercent:
                case kIntEm:
                case kIntEx:
                case kIntLH:
                case kIntGridLine:
                case kIntRem:
                case kIntCh:
                case kIntVW:
                case kIntVH:
                case kIntVMin:
                case kIntVMax:
                case kIntUnsigned:
                case kParameterNumber:
                    return Long.toString(value.d());
                case kList: {
                    List<String> values = new ArrayList<>();
                    for (ac item : value.h()) values.add(value(book, item, depth + 1));
                    return "[" + String.join(", ", values) + "]";
                }
                case kPropList: {
                    Z props = value.g();
                    if (props == null) return "{}";
                    List<String> values = new ArrayList<>();
                    for (ab p : props.a()) values.add(p.a() + "=" + value(book, props.a(p), depth + 1));
                    return "{" + String.join(", ", values) + "}";
                }
                case kBlob:
                    return "blob[" + value.m().length + "]";
                default:
                    return "<" + type + ">";
            }
        } catch (Throwable t) {
            return "<ERR:" + t.getClass().getSimpleName() + ">";
        }
    }

    private static void printProperties(DigitalBook book, Container container, String indent) {
        try {
            for (ab property : container.r()) {
                ac propertyValue = container.a(property);
                System.out.println(indent + property.d() + " " + property.a() + " [" +
                        (propertyValue == null ? "null" : propertyValue.a()) + "] = " +
                        value(book, propertyValue, 0));
            }
        } catch (Throwable t) {
            System.out.println(indent + "<props ERR " + t + ">");
        }
    }

    private static void printText(DigitalBook book, Container container, String indent) {
        try {
            av text = container.o();
            if (text == null) {
                System.out.println(indent + "text=null");
                return;
            }
            TextContainer textContainer = (TextContainer) text;
            List<V> elements = textContainer.B();
            List<aq> styleEvents = textContainer.t();
            System.out.println(indent + "paragraphElements=" + elements.size() + " styleEvents=" + styleEvents.size());

            for (int i = 0; i < elements.size(); i++) {
                V paragraphElement = elements.get(i);
                System.out.print(indent + "  PE[" + i + "] kind=" + paragraphElement.a());
                try {
                    aw textElement = paragraphElement.c();
                    System.out.print(" text=" + (textElement == null ? "null" : quote(textElement.e())) +
                            " enc=" + (textElement == null ? "-" : textElement.f()) +
                            " bytes=" + (textElement == null ? "-" : textElement.g()) +
                            " chars=" + (textElement == null ? "-" : textElement.h()));
                } catch (Throwable t) {
                    System.out.print(" textERR=" + t.getClass().getSimpleName());
                }
                System.out.println();
            }

            for (aq event : styleEvents) {
                try {
                    T offset = event.a();
                    T length = event.b();
                    Z props = event.c();
                    System.out.println(indent + "  styleEvent offset=" + offset.a() + ":" + offset.b() +
                            " length=" + length.a() + ":" + length.b() + " props=" + props.a().size());
                    for (ab property : props.a()) {
                        ac propertyValue = props.a(property);
                        System.out.println(indent + "    evprop " + property.d() + " " + property.a() + " [" +
                                propertyValue.a() + "] = " + value(book, propertyValue, 0));
                    }
                } catch (Throwable t) {
                    System.out.println(indent + "  styleEvent ERR " + t.getClass().getSimpleName());
                }
            }
        } catch (Throwable t) {
            System.out.println(indent + "text ERR " + t);
        }
    }

    private static void printContainer(DigitalBook book, s source, String indent, Set<Long> seen, int depth)
            throws Exception {
        if (source == null) return;
        Container container = (Container) source;
        long id = container.a();
        System.out.println(indent + "CONTAINER name=" + quote(book.f(id)) + " id=" + id +
                " elemType=" + container.b() + " containerType=" + container.h() + " eid=" + container.i());
        printProperties(book, container, indent + "  prop ");
        if (container.h() == aB.m.TEXT) printText(book, container, indent + "  ");
        if (!seen.add(id) || depth > 6) return;
        for (s child : container.m()) printContainer(book, child, indent + "  ", seen, depth + 1);
    }

    public static void main(String[] args) throws Exception {
        if (args.length != 1) {
            System.err.println("usage: KafSemanticProbe <book.kdf>");
            System.exit(2);
        }

        c.a();
        BookFactory factory = new BookFactory();
        factory.b();
        DigitalBook book = (DigitalBook) factory.a(args[0]);
        BookContent content = (BookContent) book.d();

        System.out.println("-- document data --");
        DocumentData documentData = (DocumentData) content.a();
        for (ab property : documentData.a()) {
            ac propertyValue = documentData.a(property);
            System.out.println(property.d() + " " + property.a() + " [" + propertyValue.a() + "] = " +
                    value(book, propertyValue, 0));
        }

        System.out.println("-- styles --");
        for (String name : content.a(aB.o.Style)) {
            Style style = (Style) content.g(book.a(name));
            System.out.println("STYLE " + quote(name) + " id=" + style.a() + " type=" + style.b());
            for (ab property : style.g()) {
                ac propertyValue = style.a(property);
                System.out.println("  " + property.d() + " " + property.a() + " [" + propertyValue.a() + "] = " +
                        value(book, propertyValue, 0));
            }
        }

        System.out.println("-- storylines --");
        for (String name : content.a(aB.o.Storyline)) {
            Storyline storyline = (Storyline) content.d(book.a(name));
            System.out.println("STORY " + quote(name) + " id=" + storyline.a() + " children=" + storyline.g().size());
            for (s item : storyline.g()) printContainer(book, item, "  ", new HashSet<Long>(), 0);
        }

        // Avoid normal teardown: some JNI adapter ownership paths in this old bundled
        // runtime are unsafe to explore indiscriminately. This probe is intentionally
        // a one-shot subprocess.
        System.out.flush();
        System.exit(0);
    }
}
