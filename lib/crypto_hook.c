#define _GNU_SOURCE
#include <dlfcn.h>
#include <pthread.h>
#include <stdio.h>

/*
 * Capture the AES-256-CBC key that Amazon's YJ SDK passes to OpenSSL while
 * decrypting a voucher.  Do not use dlsym(RTLD_NEXT) here: on Bellatrix3 the
 * SDK/JNI stack is loaded into a local dlopen scope, so libcrypto is not in the
 * preloaded hook's RTLD_NEXT search scope.  Resolve against libcrypto itself
 * instead.
 */

typedef int (*evp_decrypt_init_t)(
    void *, const void *, void *, const unsigned char *, const unsigned char *);
typedef const void *(*cipher_factory_t)(void);

static pthread_once_t crypto_once = PTHREAD_ONCE_INIT;
static pthread_mutex_t log_mutex = PTHREAD_MUTEX_INITIALIZER;
static evp_decrypt_init_t real_decrypt_init = NULL;
static const void *aes256_cbc = NULL;

static FILE *open_log(void) {
    return fopen("/mnt/us/crypto_keys.log", "a");
}

static void load_crypto_symbols(void) {
    static const char *const sonames[] = {
        "libcrypto.so.3",
        "libcrypto.so.1.1",
        "libcrypto.so.1.0.0",
        "libcrypto.so",
        NULL,
    };

    void *crypto = NULL;
    for (size_t i = 0; sonames[i] != NULL; i++) {
        crypto = dlopen(sonames[i], RTLD_LAZY | RTLD_LOCAL);
        if (crypto != NULL) {
            break;
        }
    }
    if (crypto == NULL) {
        return;
    }

    real_decrypt_init = (evp_decrypt_init_t)dlsym(crypto, "EVP_DecryptInit_ex");
    cipher_factory_t aes256_factory =
        (cipher_factory_t)dlsym(crypto, "EVP_aes_256_cbc");
    if (aes256_factory != NULL) {
        aes256_cbc = aes256_factory();
    }

    /* Keep the handle open for the lifetime of the process. */
}

int EVP_DecryptInit_ex(void *ctx, const void *type, void *impl,
                       const unsigned char *key, const unsigned char *iv) {
    pthread_once(&crypto_once, load_crypto_symbols);

    /* Failing the OpenSSL operation is safer than calling a null function
     * pointer if a future firmware ships an unsupported libcrypto ABI. */
    if (real_decrypt_init == NULL) {
        return 0;
    }

    if (key != NULL && type == aes256_cbc) {
        pthread_mutex_lock(&log_mutex);
        FILE *f = open_log();
        if (f != NULL) {
            fprintf(f, "EVP_256_KEY:");
            for (int i = 0; i < 32; i++) {
                fprintf(f, "%02x", key[i]);
            }
            fprintf(f, " IV:");
            if (iv != NULL) {
                for (int i = 0; i < 16; i++) {
                    fprintf(f, "%02x", iv[i]);
                }
            } else {
                fprintf(f, "none");
            }
            fprintf(f, "\n");
            fflush(f);
            fclose(f);
        }
        pthread_mutex_unlock(&log_mutex);
    }

    return real_decrypt_init(ctx, type, impl, key, iv);
}
