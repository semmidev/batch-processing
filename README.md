# Batch Processing Middleware

Middleware pemrosesan batch berkelas enterprise yang sangat tangguh, ditulis dalam Go. Layanan ini berfungsi sebagai lapisan perantara yang menerima kumpulan pekerjaan dalam jumlah besar dari berbagai sistem sumber, memprosesnya secara bersamaan (concurrent) terhadap sistem eksternal secara aman, serta mengirimkan status penyelesaian melalui webhook yang terenkripsi.

## Fitur Utama

- **Konkurensi Kuat**: Memanfaatkan fitur `FOR UPDATE SKIP LOCKED` milik PostgreSQL 18 untuk konkurensi antar baris yang solid dan bebas deadlock di seluruh worker node yang di-*scale* secara horizontal.
- **API Idempoten**: Semua endpoint pembuatan batch mewajibkan header `Idempotency-Key` untuk menangani percobaan ulang jaringan secara aman dan mencegah pemrosesan batch duplikat.
- **Webhook Tangguh (Outbox Pattern)**: Webhook dikirimkan menggunakan pola *Transactional Outbox*. Jika tujuan tidak dapat dijangkau, *exponential backoff* akan diterapkan secara otomatis.
- **Dead Letter Queue (DLQ)**: Item yang gagal dan telah menghabiskan jumlah maksimum percobaan ulang akan disimpan dengan aman ke dalam Dead Letter Queue untuk inspeksi manual dan pemutaran ulang.
- **Pemulihan Kunci Basi (Stale Lock Recovery)**: Secara otomatis mendeteksi dan memulihkan item yang tertinggal dalam status "processing" akibat worker node yang crash atau tidak merespons.
- **Identifier Modern**: Menggunakan `uuidv7()` bawaan PostgreSQL 18 untuk semua primary key, memastikan lokalitas yang terurut berdasarkan waktu dan skalabilitas tinggi.
- **Observabilitas**: Metrik Prometheus bawaan dan logging JSON terstruktur menggunakan `go.uber.org/zap`.
- **Webhook Aman**: Payload webhook ditandatangani menggunakan HMAC SHA-256, memungkinkan konsumen memverifikasi keaslian payload webhook yang diterima.

## Teknologi yang Digunakan

- **Bahasa**: Go 1.21+
- **Database**: PostgreSQL 18
- **Router**: `go-chi/chi`
- **Driver/Mapper Database**: `github.com/lib/pq` & `github.com/jmoiron/sqlx`
- **Migrasi**: `golang-migrate/migrate`
- **Logging**: `go.uber.org/zap`

## Struktur Proyek

```
.
├── cmd/
│   └── server/             # Titik masuk utama aplikasi
├── internal/
│   ├── config/             # Loader konfigurasi aplikasi (viper/env)
│   ├── database/           # Inisialisasi koneksi PostgreSQL
│   ├── domain/             # Model domain inti dan enumerasi
│   ├── adapter/
│   │   ├── driven/         # Adapter keluar (repositori, klien System C)
│   │   └── driving/        # Adapter masuk (HTTP handler, worker pool)
│   ├── application/        # Implementasi use case bisnis
│   ├── observability/      # Logger Zap & Metrik Prometheus
│   ├── port/
│   │   ├── input/          # Interface use case (port masuk)
│   │   └── output/         # Interface repositori/klien (port keluar)
│   └── uuid/               # Package pembuat UUID v7
├── migrations/             # File migrasi up/down PostgreSQL (.sql)
├── simulators/             # Program simulator System A dan System C
├── docker-compose.yml      # Infrastruktur pengembangan lokal
├── Makefile                # Perintah build, run, dan migrasi
└── .env.example            # Contoh variabel lingkungan
```

## Memulai

### Prasyarat
- Go 1.21+
- Docker & Docker Compose
- CLI `golang-migrate` (opsional, jika menjalankan migrasi secara manual)

### 1. Pengaturan Lingkungan

Salin file lingkungan contoh:
```bash
cp .env.example .env
```

### 2. Jalankan Semua Layanan (Cara Termudah)

Cukup jalankan perintah berikut di direktori root proyek. Perintah ini akan otomatis menjalankan database, migrasi skema, middleware, dan kedua simulator:
```bash
docker-compose up --build
```

### 3. Jalankan Migrasi (Opsional, jika tanpa Docker)

Pastikan skema database sudah terkini:
```bash
make migrate-up
```

### 4. Jalankan Aplikasi (Opsional, jika tanpa Docker)

Mulai API dan Background Worker:
```bash
make run
```
Secara default, API akan tersedia di `http://localhost:8080`.

## Dokumentasi API

### Buat Batch Baru
`POST /api/v1/batches`

Header:
- `Authorization: Bearer <api-key>` (Wajib): Kunci API untuk autentikasi.
- `X-Idempotency-Key` (Wajib): Kunci unik untuk mencegah pemrosesan duplikat.

Body:
```json
{
  "correlation_id": "req-12345",
  "webhook_url": "https://api.example.com/webhooks/batches",
  "items": [
    {
      "external_id": "invoice_778",
      "payload": {"amount": 100.00, "currency": "USD"}
    }
  ]
}
```

### Batalkan Batch
`POST /api/v1/batches/{batch_id}/cancel`

Membatalkan sebuah batch. Semua item yang saat ini berstatus `pending` akan ditandai sebagai `cancelled`.

### Payload Webhook Status Batch
Ketika sebuah batch selesai diproses (berhasil, berhasil sebagian, atau gagal), permintaan POST akan dikirimkan ke `webhook_url` yang dikonfigurasi dengan header `X-Signature` (HMAC SHA-256) untuk verifikasi.

```json
{
  "event": "batch_completed",
  "batch_id": "019e447b-...-uuidv7",
  "correlation_id": "req-12345",
  "status": "done",
  "summary": {
    "total": 1,
    "success": 1,
    "failed": 0
  },
  "failed_items": [],
  "timestamp": "2026-05-20T12:00:00Z"
}
```

Kemungkinan nilai `status`:
- `done` — Semua item berhasil diproses.
- `partial` — Sebagian item berhasil, sebagian gagal dan masuk DLQ.
- `failed` — Semua item gagal.

## Lisensi
MIT License
