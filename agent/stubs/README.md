# Kindle ReaderSDK compile-time stubs

These classes contain **signatures only**. They exist so `scripts/build_progress_agent`
can compile the reading-progress Java agent without redistributing Amazon firmware
JARs.

The package names, method descriptors, and field names match the Kindle ReaderSDK
API used by `KindlePluginReadingProgressAgentV6`. The stubs are never included in
the agent JAR or release package; at runtime the attached agent resolves the real
classes from the Kindle framework JVM.

When changing the native bridge, verify every changed descriptor against device
firmware (or a legally obtained local class dump) before updating these stubs.
