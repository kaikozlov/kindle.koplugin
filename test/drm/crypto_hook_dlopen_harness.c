/*
 * Reproduce the loader topology used by newer Kindle firmware: the main
 * process does not link libcrypto directly; it dlopens an SDK-like helper into
 * a local scope, and that helper calls EVP_DecryptInit_ex.  A preload hook that
 * forwards through RTLD_NEXT cannot see this local libcrypto dependency on
 * Bellatrix3.  Resolving through a libcrypto handle must still work.
 */

#ifdef CRYPTO_HOOK_DLOPEN_HELPER

#include <openssl/evp.h>

int run_crypto_hook_test(void) {
    unsigned char key[32];
    unsigned char iv[16];
    for (int i = 0; i < 32; i++) key[i] = (unsigned char)i;
    for (int i = 0; i < 16; i++) iv[i] = (unsigned char)(0xa0 + i);

    EVP_CIPHER_CTX *ctx = EVP_CIPHER_CTX_new();
    if (ctx == NULL) return 1;
    const EVP_CIPHER *cipher = EVP_aes_256_cbc();
    if (cipher == NULL || EVP_DecryptInit_ex(ctx, cipher, NULL, key, iv) != 1) {
        EVP_CIPHER_CTX_free(ctx);
        return 1;
    }
    EVP_CIPHER_CTX_free(ctx);
    return 0;
}

#else

#define _GNU_SOURCE
#include <dlfcn.h>
#include <stdio.h>

typedef int (*run_test_t)(void);

int main(void) {
    void *helper = dlopen("./crypto_hook_dlopen_helper.so", RTLD_NOW | RTLD_LOCAL);
    if (helper == NULL) {
        fprintf(stderr, "dlopen helper failed: %s\n", dlerror());
        return 1;
    }
    run_test_t run_test = (run_test_t)dlsym(helper, "run_crypto_hook_test");
    if (run_test == NULL) {
        fprintf(stderr, "dlsym helper failed: %s\n", dlerror());
        return 1;
    }
    int result = run_test();
    dlclose(helper);
    return result;
}

#endif
