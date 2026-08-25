/*
 * crypto_hook_harness.c — executable test for the shipped crypto_hook.so.
 *
 * Allocates an OpenSSL cipher context and initializes an AES-256-CBC decrypt
 * operation with fixed key bytes 00..1f and IV bytes a0..af. When run with
 * LD_PRELOAD=crypto_hook.so, the hook must intercept EVP_DecryptInit_ex via
 * dlsym(RTLD_NEXT), recognize the EVP_aes_256_cbc() cipher pointer, capture
 * the 32-byte key and 16-byte IV, and append the exact record
 *
 *   EVP_256_KEY:000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f
 *   IV:a0a1a2a3a4a5a6a7a8a9aaabacadaeaf
 *
 * to /mnt/us/crypto_keys.log. The Dockerfile.crypto_hook test targets compile
 * this harness for ARMv7 and assert the log contents; this file only fails on
 * OpenSSL errors.
 */

#include <openssl/evp.h>
#include <stdio.h>

int main(void) {
    unsigned char key[32];
    unsigned char iv[16];
    for (int i = 0; i < 32; i++) {
        key[i] = (unsigned char)i;          /* 00 01 ... 1f */
    }
    for (int i = 0; i < 16; i++) {
        iv[i] = (unsigned char)(0xa0 + i);  /* a0 a1 ... af */
    }

    EVP_CIPHER_CTX *ctx = EVP_CIPHER_CTX_new();
    if (ctx == NULL) {
        fprintf(stderr, "crypto_hook_harness: EVP_CIPHER_CTX_new failed\n");
        return 1;
    }

    const EVP_CIPHER *cipher = EVP_aes_256_cbc();
    if (cipher == NULL) {
        fprintf(stderr, "crypto_hook_harness: EVP_aes_256_cbc failed\n");
        EVP_CIPHER_CTX_free(ctx);
        return 1;
    }

    if (EVP_DecryptInit_ex(ctx, cipher, NULL, key, iv) != 1) {
        fprintf(stderr, "crypto_hook_harness: EVP_DecryptInit_ex failed\n");
        EVP_CIPHER_CTX_free(ctx);
        return 1;
    }

    EVP_CIPHER_CTX_free(ctx);
    printf("crypto_hook_harness: EVP_DecryptInit_ex(aes-256-cbc) ok\n");
    return 0;
}
