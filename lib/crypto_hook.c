#define _GNU_SOURCE
#include <dlfcn.h>
#include <stdio.h>
#include <string.h>
#include <pthread.h>

static pthread_mutex_t log_mutex = PTHREAD_MUTEX_INITIALIZER;
static const void *aes128_ptr = NULL;
static const void *aes256_ptr = NULL;
static int hook_enabled = 1;

static FILE* open_log(void) {
    return fopen("/mnt/us/crypto_keys.log", "a");
}

static void hexdump(FILE *f, const unsigned char *data, int len) {
    for (int i = 0; i < len; i++) fprintf(f, "%02x", data[i]);
}

static void ensure_ptrs(void) {
    if (!aes128_ptr) {
        void *(*fn)(void) = (void *(*)(void))dlsym(RTLD_DEFAULT, "EVP_aes_128_cbc");
        if (fn) aes128_ptr = fn();
    }
    if (!aes256_ptr) {
        void *(*fn)(void) = (void *(*)(void))dlsym(RTLD_DEFAULT, "EVP_aes_256_cbc");
        if (fn) aes256_ptr = fn();
    }
}

/* ========================================================================
 * Original AES hooks
 * ======================================================================== */

/* Hook EVP_DecryptInit_ex */
typedef int (*evp_decrypt_init_t)(void*, const void*, void*, const unsigned char*, const unsigned char*);

int EVP_DecryptInit_ex(void *ctx, const void *type, void *impl,
                       const unsigned char *key, const unsigned char *iv) {
    static evp_decrypt_init_t real = NULL;
    if (!real) real = (evp_decrypt_init_t)dlsym(RTLD_NEXT, "EVP_DecryptInit_ex");
    
    ensure_ptrs();
    
    if (hook_enabled && key && (type == aes128_ptr || type == aes256_ptr)) {
        pthread_mutex_lock(&log_mutex);
        FILE *f = open_log();
        if (f) {
            int keylen = (type == aes256_ptr) ? 32 : 16;
            fprintf(f, "EVP_DEC_%d_KEY:", keylen * 8);
            hexdump(f, key, keylen);
            fprintf(f, " IV:");
            if (iv) hexdump(f, iv, 16);
            else fprintf(f, "none");
            fprintf(f, "\n");
            fflush(f);
            fclose(f);
        }
        pthread_mutex_unlock(&log_mutex);
    }
    return real(ctx, type, impl, key, iv);
}

/* Hook EVP_EncryptInit_ex */
typedef int (*evp_encrypt_init_t)(void*, const void*, void*, const unsigned char*, const unsigned char*);

int EVP_EncryptInit_ex(void *ctx, const void *type, void *impl,
                       const unsigned char *key, const unsigned char *iv) {
    static evp_encrypt_init_t real = NULL;
    if (!real) real = (evp_encrypt_init_t)dlsym(RTLD_NEXT, "EVP_EncryptInit_ex");
    
    ensure_ptrs();
    
    if (hook_enabled && key && (type == aes128_ptr || type == aes256_ptr)) {
        pthread_mutex_lock(&log_mutex);
        FILE *f = open_log();
        if (f) {
            int keylen = (type == aes256_ptr) ? 32 : 16;
            fprintf(f, "EVP_ENC_%d_KEY:", keylen * 8);
            hexdump(f, key, keylen);
            fprintf(f, " IV:");
            if (iv) hexdump(f, iv, 16);
            else fprintf(f, "none");
            fprintf(f, "\n");
            fflush(f);
            fclose(f);
        }
        pthread_mutex_unlock(&log_mutex);
    }
    return real(ctx, type, impl, key, iv);
}

/* Hook AES_set_decrypt_key */
typedef int (*aes_set_dec_key_t)(const unsigned char *, int, void*);

int AES_set_decrypt_key(const unsigned char *key, int bits, void *aes_key) {
    static aes_set_dec_key_t real = NULL;
    if (!real) real = (aes_set_dec_key_t)dlsym(RTLD_NEXT, "AES_set_decrypt_key");
    
    if (hook_enabled && key && (bits == 128 || bits == 256)) {
        pthread_mutex_lock(&log_mutex);
        FILE *f = open_log();
        if (f) {
            fprintf(f, "AES_DEC_%d_KEY:", bits);
            hexdump(f, key, bits/8);
            fprintf(f, "\n");
            fflush(f);
            fclose(f);
        }
        pthread_mutex_unlock(&log_mutex);
    }
    return real(key, bits, aes_key);
}

/* Hook AES_set_encrypt_key */
typedef int (*aes_set_enc_key_t)(const unsigned char *, int, void*);

int AES_set_encrypt_key(const unsigned char *key, int bits, void *aes_key) {
    static aes_set_enc_key_t real = NULL;
    if (!real) real = (aes_set_enc_key_t)dlsym(RTLD_NEXT, "AES_set_encrypt_key");
    
    if (hook_enabled && key && (bits == 128 || bits == 256)) {
        pthread_mutex_lock(&log_mutex);
        FILE *f = open_log();
        if (f) {
            fprintf(f, "AES_ENC_%d_KEY:", bits);
            hexdump(f, key, bits/8);
            fprintf(f, "\n");
            fflush(f);
            fclose(f);
        }
        pthread_mutex_unlock(&log_mutex);
    }
    return real(key, bits, aes_key);
}

/* ========================================================================
 * HMAC hooks — capture full HMAC(key, data) → md
 * ======================================================================== */

typedef unsigned char *(*hmac_t)(const void *, const void *, size_t,
                                  const unsigned char *, size_t,
                                  unsigned char *, unsigned int *);

unsigned char *HMAC(const void *evp_md, const void *key, int key_len,
                    const unsigned char *data, size_t data_len,
                    unsigned char *md, unsigned int *md_len) {
    static hmac_t real = NULL;
    if (!real) real = (hmac_t)dlsym(RTLD_NEXT, "HMAC");
    
    unsigned char *result = real(evp_md, key, key_len, data, data_len, md, md_len);
    
    if (hook_enabled && result && evp_md) {
        pthread_mutex_lock(&log_mutex);
        FILE *f = open_log();
        if (f) {
            int outlen = md_len ? (int)*md_len : 32; /* default SHA256 */
            fprintf(f, "HMAC(key=%d:", key_len);
            if (key && key_len > 0) hexdump(f, (const unsigned char*)key, key_len > 64 ? 64 : key_len);
            fprintf(f, " data=%d:", (int)data_len);
            if (data && data_len > 0) hexdump(f, data, data_len > 128 ? 128 : data_len);
            fprintf(f, " md=%d:", outlen);
            hexdump(f, result, outlen);
            fprintf(f, ")\n");
            fflush(f);
            fclose(f);
        }
        pthread_mutex_unlock(&log_mutex);
    }
    return result;
}

/* Also hook HMAC_Init_ex / HMAC_Update / HMAC_Final for incremental HMAC */
typedef int (*hmac_init_ex_t)(void *ctx, const void *, int, const void *, void *);

int HMAC_Init_ex(void *ctx, const void *key, int key_len,
                 const void *md, void *impl) {
    static hmac_init_ex_t real = NULL;
    if (!real) real = (hmac_init_ex_t)dlsym(RTLD_NEXT, "HMAC_Init_ex");
    
    if (hook_enabled && key && key_len > 0) {
        pthread_mutex_lock(&log_mutex);
        FILE *f = open_log();
        if (f) {
            fprintf(f, "HMAC_INIT(key=%d:", key_len);
            hexdump(f, (const unsigned char*)key, key_len > 64 ? 64 : key_len);
            fprintf(f, ")\n");
            fflush(f);
            fclose(f);
        }
        pthread_mutex_unlock(&log_mutex);
    }
    return real(ctx, key, key_len, md, impl);
}

typedef int (*hmac_update_t)(void *ctx, const unsigned char *, size_t);

int HMAC_Update(void *ctx, const unsigned char *data, size_t len) {
    static hmac_update_t real = NULL;
    if (!real) real = (hmac_update_t)dlsym(RTLD_NEXT, "HMAC_Update");
    
    if (hook_enabled && data && len > 0) {
        pthread_mutex_lock(&log_mutex);
        FILE *f = open_log();
        if (f) {
            fprintf(f, "HMAC_UPDATE(data=%d:", (int)len);
            hexdump(f, data, len > 128 ? 128 : len);
            fprintf(f, ")\n");
            fflush(f);
            fclose(f);
        }
        pthread_mutex_unlock(&log_mutex);
    }
    return real(ctx, data, len);
}

typedef int (*hmac_final_t)(void *ctx, unsigned char *, unsigned int *);

int HMAC_Final(void *ctx, unsigned char *md, unsigned int *len) {
    static hmac_final_t real = NULL;
    if (!real) real = (hmac_final_t)dlsym(RTLD_NEXT, "HMAC_Final");
    
    int ret = real(ctx, md, len);
    
    if (hook_enabled && ret && md && len) {
        pthread_mutex_lock(&log_mutex);
        FILE *f = open_log();
        if (f) {
            fprintf(f, "HMAC_FINAL(md=%d:", *len);
            hexdump(f, md, *len);
            fprintf(f, ")\n");
            fflush(f);
            fclose(f);
        }
        pthread_mutex_unlock(&log_mutex);
    }
    return ret;
}

/* ========================================================================
 * SHA256 hook — catch direct SHA256() calls
 * ======================================================================== */

typedef unsigned char *(*sha256_t)(const unsigned char *, size_t, unsigned char *);

unsigned char *SHA256(const unsigned char *data, size_t len, unsigned char *md) {
    static sha256_t real = NULL;
    if (!real) real = (sha256_t)dlsym(RTLD_NEXT, "SHA256");
    
    unsigned char *result = real(data, len, md);
    
    if (hook_enabled && result) {
        pthread_mutex_lock(&log_mutex);
        FILE *f = open_log();
        if (f) {
            fprintf(f, "SHA256(data=%d:", (int)len);
            if (data && len > 0) hexdump(f, data, len > 128 ? 128 : len);
            fprintf(f, " hash=");
            hexdump(f, result, 32);
            fprintf(f, ")\n");
            fflush(f);
            fclose(f);
        }
        pthread_mutex_unlock(&log_mutex);
    }
    return result;
}

/* ========================================================================
 * EVP_Digest hooks — catch EVP-based hashing (SHA256 via EVP)
 * ======================================================================== */

typedef int (*evp_digest_init_t)(void *ctx, const void *type);

int EVP_DigestInit(void *ctx, const void *type) {
    static evp_digest_init_t real = NULL;
    if (!real) real = (evp_digest_init_t)dlsym(RTLD_NEXT, "EVP_DigestInit");
    return real(ctx, type);
}

typedef int (*evp_digest_update_t)(void *ctx, const void *, size_t);

int EVP_DigestUpdate(void *ctx, const void *data, size_t len) {
    static evp_digest_update_t real = NULL;
    if (!real) real = (evp_digest_update_t)dlsym(RTLD_NEXT, "EVP_DigestUpdate");
    
    if (hook_enabled && data && len > 0) {
        pthread_mutex_lock(&log_mutex);
        FILE *f = open_log();
        if (f) {
            fprintf(f, "DIGEST_UPDATE(data=%d:", (int)len);
            hexdump(f, (const unsigned char*)data, len > 128 ? 128 : len);
            fprintf(f, ")\n");
            fflush(f);
            fclose(f);
        }
        pthread_mutex_unlock(&log_mutex);
    }
    return real(ctx, data, len);
}

typedef int (*evp_digest_final_t)(void *ctx, unsigned char *, unsigned int *);

int EVP_DigestFinal(void *ctx, unsigned char *md, unsigned int *len) {
    static evp_digest_final_t real = NULL;
    if (!real) real = (evp_digest_final_t)dlsym(RTLD_NEXT, "EVP_DigestFinal");
    
    int ret = real(ctx, md, len);
    
    if (hook_enabled && ret && md && len) {
        pthread_mutex_lock(&log_mutex);
        FILE *f = open_log();
        if (f) {
            fprintf(f, "DIGEST_FINAL(md=%d:", *len);
            hexdump(f, md, *len);
            fprintf(f, ")\n");
            fflush(f);
            fclose(f);
        }
        pthread_mutex_unlock(&log_mutex);
    }
    return ret;
}

typedef int (*evp_digest_final_ex_t)(void *ctx, unsigned char *, unsigned int *);

int EVP_DigestFinal_ex(void *ctx, unsigned char *md, unsigned int *len) {
    static evp_digest_final_ex_t real = NULL;
    if (!real) real = (evp_digest_final_ex_t)dlsym(RTLD_NEXT, "EVP_DigestFinal_ex");
    
    int ret = real(ctx, md, len);
    
    if (hook_enabled && ret && md && len) {
        pthread_mutex_lock(&log_mutex);
        FILE *f = open_log();
        if (f) {
            fprintf(f, "DIGEST_FINAL_EX(md=%d:", *len);
            hexdump(f, md, *len);
            fprintf(f, ")\n");
            fflush(f);
            fclose(f);
        }
        pthread_mutex_unlock(&log_mutex);
    }
    return ret;
}
