#define _GNU_SOURCE
#include <dlfcn.h>
#include <stdio.h>
#include <string.h>
#include <pthread.h>

static pthread_mutex_t log_mutex = PTHREAD_MUTEX_INITIALIZER;
static const void *aes128_ptr = NULL;
static const void *aes256_ptr = NULL;

static FILE* open_log(void) {
    return fopen("/mnt/us/crypto_keys.log", "a");
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

/* Hook EVP_DecryptInit_ex */
typedef int (*evp_decrypt_init_t)(void*, const void*, void*, const unsigned char*, const unsigned char*);

int EVP_DecryptInit_ex(void *ctx, const void *type, void *impl,
                       const unsigned char *key, const unsigned char *iv) {
    static evp_decrypt_init_t real = NULL;
    if (!real) real = (evp_decrypt_init_t)dlsym(RTLD_NEXT, "EVP_DecryptInit_ex");
    
    ensure_ptrs();
    
    if (key && (type == aes128_ptr || type == aes256_ptr)) {
        pthread_mutex_lock(&log_mutex);
        FILE *f = open_log();
        if (f) {
            int keylen = (type == aes256_ptr) ? 32 : 16;
            fprintf(f, "EVP_%d_KEY:", keylen * 8);
            for (int i = 0; i < keylen; i++) fprintf(f, "%02x", key[i]);
            fprintf(f, " IV:");
            if (iv) { for (int i = 0; i < 16; i++) fprintf(f, "%02x", iv[i]); }
            else fprintf(f, "none");
            fprintf(f, "\n");
            fflush(f);
            fclose(f);
        }
        pthread_mutex_unlock(&log_mutex);
    }
    return real(ctx, type, impl, key, iv);
}

/* Hook EVP_EncryptInit_ex too (key derivation might use encrypt) */
typedef int (*evp_encrypt_init_t)(void*, const void*, void*, const unsigned char*, const unsigned char*);

int EVP_EncryptInit_ex(void *ctx, const void *type, void *impl,
                       const unsigned char *key, const unsigned char *iv) {
    static evp_encrypt_init_t real = NULL;
    if (!real) real = (evp_encrypt_init_t)dlsym(RTLD_NEXT, "EVP_EncryptInit_ex");
    
    ensure_ptrs();
    
    if (key && (type == aes128_ptr || type == aes256_ptr)) {
        pthread_mutex_lock(&log_mutex);
        FILE *f = open_log();
        if (f) {
            int keylen = (type == aes256_ptr) ? 32 : 16;
            fprintf(f, "EVP_ENC_%d_KEY:", keylen * 8);
            for (int i = 0; i < keylen; i++) fprintf(f, "%02x", key[i]);
            fprintf(f, " IV:");
            if (iv) { for (int i = 0; i < 16; i++) fprintf(f, "%02x", iv[i]); }
            else fprintf(f, "none");
            fprintf(f, "\n");
            fflush(f);
            fclose(f);
        }
        pthread_mutex_unlock(&log_mutex);
    }
    return real(ctx, type, impl, key, iv);
}

/* Hook AES_set_decrypt_key - low level AES API */
typedef int (*aes_set_dec_key_t)(const unsigned char *, int, void*);

int AES_set_decrypt_key(const unsigned char *key, int bits, void *aes_key) {
    static aes_set_dec_key_t real = NULL;
    if (!real) real = (aes_set_dec_key_t)dlsym(RTLD_NEXT, "AES_set_decrypt_key");
    
    if (key && (bits == 128 || bits == 256)) {
        pthread_mutex_lock(&log_mutex);
        FILE *f = open_log();
        if (f) {
            fprintf(f, "AES_DEC_%d_KEY:", bits);
            for (int i = 0; i < bits/8; i++) fprintf(f, "%02x", key[i]);
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
    
    if (key && (bits == 128 || bits == 256)) {
        pthread_mutex_lock(&log_mutex);
        FILE *f = open_log();
        if (f) {
            fprintf(f, "AES_ENC_%d_KEY:", bits);
            for (int i = 0; i < bits/8; i++) fprintf(f, "%02x", key[i]);
            fprintf(f, "\n");
            fflush(f);
            fclose(f);
        }
        pthread_mutex_unlock(&log_mutex);
    }
    return real(key, bits, aes_key);
}
