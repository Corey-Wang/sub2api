# Encrypted ChatGPT / OpenAI account import

On-prem customers can batch-import ChatGPT (OpenAI / Codex OAuth) accounts **without seeing plaintext secrets**. The seller encrypts account records to the instance import public key. Only the sub2api server process holds the matching private key.

This is operator-vs-on-site-staff concealment, not DRM. A user with root or database access can still dump stored credentials.

## Package format (version 1)

File kind: `sub2api.encrypted_account_bundle`

```json
{
  "kind": "sub2api.encrypted_account_bundle",
  "version": 1,
  "alg": "x25519-hkdf-sha256-xchacha20poly1305",
  "recipient_kid": "sha256:<hex of recipient public key>",
  "created_at": "2026-08-14T00:00:00Z",
  "items": [
    {
      "id": "seller-stable-id",
      "ephemeral_public_key": "<raw-url-base64 32 bytes>",
      "nonce": "<raw-url-base64 24 bytes>",
      "ciphertext": "<raw-url-base64 XChaCha20-Poly1305>"
    }
  ]
}
```

Each item is independently encrypted so one tampered row can fail without discarding the rest of the batch.

**Key agreement:** ephemeral X25519 × recipient X25519  
**KDF:** HKDF-SHA256, info `sub2api-account-import-v1`, salt `recipient_pub || ephemeral_pub`  
**AEAD:** XChaCha20-Poly1305, AAD `v1|<kid>|<item id>`

Plaintext inside each item:

```json
{
  "version": 1,
  "id": "seller-stable-id",
  "platform": "openai",
  "type": "oauth",
  "name": "optional-alias",
  "credentials": {
    "access_token": "...",
    "refresh_token": "...",
    "id_token": "...",
    "chatgpt_account_id": "...",
    "chatgpt_user_id": "..."
  }
}
```

`platform` must be `openai` / `chatgpt` / `codex` (or omitted). Codex `auth.json` / session objects are also accepted by the seller CLI (`access_token` / `tokens.refresh_token` at the top level).

After decrypt, sub2api reuses the existing OpenAI/Codex OAuth persistence path. Imported accounts are immediately schedulable like accounts added in the admin UI.

## Generate the instance keypair

On the **customer** server (once, at setup):

```bash
cd backend
go run ./cmd/encrypt-accounts gen-keypair
```

The command prints:

```
ACCOUNT_IMPORT_PRIVATE_KEY=<raw-url-base64>
public_key=<raw-url-base64>
kid=sha256:...
```

Put **only** the private key in server config:

```yaml
account_import:
  private_key: "<ACCOUNT_IMPORT_PRIVATE_KEY>"
```

or environment:

```bash
ACCOUNT_IMPORT_PRIVATE_KEY=...
```

Restart sub2api. Never put the private key in the frontend, a ticket, or a file handed to on-site staff.

The admin UI (账号管理 → 加密导入) shows the derived public key and `kid`. The seller only needs that public key.

## Seller export

Seller-side plaintext file (fake example: `docs/fixtures/encrypted-account-import.sample.json`):

```json
[
  {
    "id": "desk-001",
    "name": "门店A-1",
    "access_token": "eyJhbGciOiJub25lIn0.e30.",
    "refresh_token": "rt-fake-not-a-real-secret",
    "chatgpt_account_id": "acct-fake-1",
    "chatgpt_user_id": "user-fake-1"
  }
]
```

Encrypt to the customer's public key:

```bash
go run ./cmd/encrypt-accounts encrypt \
  --public-key "$CUSTOMER_PUBLIC_KEY" \
  --in accounts.json \
  --out bundle.json
```

`--public-key` also accepts `@pubkey.txt`. Hand `bundle.json` to the customer. Do not send the plaintext file.

Inspect metadata without decrypting:

```bash
go run ./cmd/encrypt-accounts inspect --in bundle.json
```

## Admin import (customer site)

1. Sign in as an admin (simple mode or standard mode; billing is not required).
2. Open **账号管理**.
3. Click **加密导入** (also under 更多操作).
4. Copy the instance public key if the seller still needs it.
5. Upload `bundle.json` and/or paste the JSON.
6. Click **开始导入**.
7. Review the per-item report: created / updated / skipped / failed.

Idempotency: the same `id` (or the same ChatGPT user / account identity used by Codex import) updates the existing row instead of creating a duplicate.

The list/detail API redacts tokens, cookies, and email. Encrypted-imported rows show a “加密包导入” badge instead of an email. Data export of those rows also strips secrets.

## Manual test notes

Use only fake secrets.

1. `go run ./cmd/encrypt-accounts gen-keypair`
2. Set `ACCOUNT_IMPORT_PRIVATE_KEY` and start the backend.
3. Encrypt `docs/fixtures/encrypted-account-import.sample.json` to the printed public key.
4. Admin → 账号管理 → 加密导入 → upload the bundle.
5. Confirm the new row is OpenAI OAuth, schedulable, badge “加密包导入”, and the network response for `GET /admin/accounts` does not contain `rt-fake` or the sample email.
6. Import the same bundle again: the item should be **updated**, not duplicated.
7. Flip one ciphertext byte and re-import: that item fails with “解密失败”, others can still succeed.
