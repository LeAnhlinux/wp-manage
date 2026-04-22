# WP Manage — Hướng Dẫn Sử Dụng

> Nền tảng quản lý VPS & WordPress chuyên nghiệp dành cho MSP và hosting provider.
> Panel: **https://panel.example.com**

---

## Mục Lục

1. [Đăng nhập & Bảo mật tài khoản](#1-đăng-nhập--bảo-mật-tài-khoản)
2. [Dashboard](#2-dashboard)
3. [Quản lý Servers](#3-quản-lý-servers)
4. [Quản lý Sites](#4-quản-lý-sites)
5. [Backup & Khôi phục](#5-backup--khôi-phục)
6. [Migration](#6-migration)
7. [Bảo mật (Security)](#7-bảo-mật-security)
8. [Giám sát (Monitoring)](#8-giám-sát-monitoring)
9. [DNS Management](#9-dns-management)
10. [Teams & Phân quyền](#10-teams--phân-quyền)
11. [Quản lý Users](#11-quản-lý-users)
12. [Cài đặt (Settings)](#12-cài-đặt-settings)
13. [Activity Log](#13-activity-log)

---

## 1. Đăng nhập & Bảo mật tài khoản

### Đăng nhập

1. Truy cập panel tại **https://panel.example.com**
2. Nhập **Email** và **Mật khẩu**
3. Nhấn **Đăng nhập**

> **Lưu ý:** Sau 10 lần đăng nhập sai liên tiếp, IP sẽ bị khóa tạm thời 5 phút.

### Xác thực 2 bước (2FA)

Nếu tài khoản đã bật 2FA, sau khi nhập đúng email/mật khẩu sẽ xuất hiện màn hình nhập **mã 6 chữ số** từ ứng dụng xác thực (Google Authenticator, Authy...).

Nếu mất điện thoại, dùng **recovery code** (8 ký tự) đã lưu khi bật 2FA.

### Bật/Tắt 2FA

1. Vào **Settings** → tab **Bảo mật**
2. Nhấn **Bật xác thực 2 bước**
3. Quét mã QR bằng ứng dụng xác thực
4. Nhập mã 6 chữ số để xác nhận
5. **Lưu lại 10 recovery codes** vào nơi an toàn

### Yêu cầu mật khẩu

- Tối thiểu 8 ký tự
- Có ít nhất 1 chữ hoa, 1 chữ thường, 1 chữ số

---

## 2. Dashboard

Dashboard hiển thị tổng quan hệ thống theo thời gian thực.

### Thống kê tổng quan

| Chỉ số | Ý nghĩa |
|--------|---------|
| **Total Servers** | Tổng số server đang quản lý |
| **Total Sites** | Tổng số WordPress site |
| **Active Sites** | Sites đang hoạt động (% online) |
| **Storage Tracked** | Tổng dung lượng đang theo dõi |

### Real-time Server Metrics

Mỗi server hiển thị:
- **CPU** — % sử dụng hiện tại
- **RAM** — % sử dụng hiện tại
- **Disk** — % sử dụng hiện tại
- Indicator xanh = đang online, xám = offline

> Dữ liệu cập nhật mỗi 30 giây qua WebSocket.

### Danh sách nhanh

- **Servers** — trạng thái, IP, CPU/RAM/Disk nhanh
- **Sites** — domain, PHP version, server, trạng thái

---

## 3. Quản lý Servers

### 3.1 Thêm Server mới

1. Vào **Servers** → nhấn **+ Add Server**
2. Điền thông tin:
   - **Tên server** (hiển thị trong panel)
   - **IP Address**
   - **SSH User** (mặc định: `root`)
   - **SSH Port** (mặc định: `22`)
   - **Xác thực:** Password hoặc SSH Key
3. Nhấn **Deploy** — panel sẽ tự động:
   - Kết nối SSH vào server
   - Cài đặt Nginx, PHP-FPM, MariaDB, Redis, Fail2Ban, UFW, Certbot, WP-CLI
   - Cài và khởi động `wp-manage-agent`
4. Theo dõi tiến trình cài đặt theo từng bước trong log

> **Yêu cầu server:** Ubuntu 22.04/24.04, tối thiểu 1GB RAM, port 22 mở.

### 3.2 Xem chi tiết Server

Nhấn vào tên server để mở trang chi tiết với các tab:

#### Tab Overview
- Thông số phần cứng (OS, CPU cores, RAM, Disk)
- Trạng thái agent và phiên bản
- Thời gian heartbeat cuối
- Biểu đồ CPU/RAM/Disk theo thời gian

#### Tab Firewall
Quản lý UFW firewall:
- Xem danh sách rule hiện tại
- **Thêm rule:** chọn Allow/Deny, protocol (TCP/UDP/both), port, source IP
- **Xóa rule:** nhấn icon xóa trên rule cần xóa
- Mọi thay đổi được áp dụng ngay lập tức

#### Tab Nginx
Quản lý cấu hình Nginx server-level:
- Xem và sửa cấu hình Nginx
- Thêm custom location blocks
- Cấu hình redirect rules
- **Rebuild Config** — tái tạo toàn bộ vhost từ DB
- **Health Check** — kiểm tra nginx/PHP-FPM/MariaDB/Redis

#### Tab PHP-FPM
Quản lý PHP-FPM pools:
- Xem pool của từng site
- Chỉnh `pm.max_children`, `pm.start_servers`, timeout
- Áp dụng thay đổi và reload PHP-FPM

#### Tab MariaDB
Quản lý database server:
- Danh sách databases và users
- Tạo/xóa database user
- Cấp/thu quyền truy cập
- Xem size từng database

#### Tab OPcache
- Bật/tắt OPcache
- Xem thống kê hit rate, memory usage
- Flush OPcache thủ công

#### Tab Redis
- Cấu hình Redis server-level
- Xem thống kê memory, hits, misses
- Flush cache

#### Tab Cron
Quản lý cron jobs hệ thống:
- Thêm cron job mới (command, schedule, user)
- Bật/tắt cron job
- Xóa cron job

#### Tab SSH Keys
Quản lý SSH public keys:
- Thêm SSH key (tên + nội dung public key)
- Xóa SSH key
- Keys được deploy vào `~/.ssh/authorized_keys` của root

#### Tab Alerts
Cấu hình cảnh báo theo ngưỡng tài nguyên:
- Tạo rule: chọn metric (CPU/RAM/Disk), ngưỡng %, kênh thông báo (Telegram/Slack/Email)
- Cấu hình webhook URL hoặc email nhận thông báo
- Đặt cooldown time giữa các cảnh báo

#### Tab Fail2Ban
Quản lý Fail2Ban:
- Xem danh sách jails đang active
- Xem IP đang bị ban
- **Unban IP** thủ công
- Chỉnh cấu hình jail (maxretry, bantime, findtime)

#### Tab Logs
Stream log theo thời gian thực:
- Nginx access/error log
- PHP-FPM log
- MariaDB log
- Chọn số dòng hiển thị cuối

### 3.3 Xóa Server

1. Vào trang chi tiết server
2. Nhấn **Delete Server**
3. Xác nhận — lưu ý: **không xóa dữ liệu** trên VPS, chỉ xóa record trong panel

---

## 4. Quản lý Sites

### 4.1 Tạo Site mới

1. Vào **Sites** → nhấn **+ Create Site**
2. Wizard tạo site gồm các bước:

**Bước 1 — Thông tin cơ bản:**
- **Domain** (ví dụ: `example.com`)
- **Server** — chọn server để deploy
- **PHP Version** — 7.4 / 8.0 / 8.1 / 8.2 / 8.3 / 8.4

**Bước 2 — WordPress:**
- **WP Admin User** — tên đăng nhập admin
- **WP Admin Email**
- **WP Admin Password**
- **Install Type:** Auto (cài WP tự động) hoặc Manual (chỉ tạo cấu trúc)

**Bước 3 — Tùy chọn:**
- **SSL** — bật để tự động cấp Let's Encrypt
- **Redis** — bật cache Redis cho site

3. Nhấn **Create** — theo dõi tiến trình tạo site (Nginx, PHP-FPM, DB, WP install, SSL)

> **Thời gian:** ~2-3 phút cho site đầy đủ với SSL.

### 4.2 Trang chi tiết Site

Nhấn vào domain để mở trang chi tiết với các tab:

#### Tab Overview
- Trạng thái site, PHP version, WordPress version
- **Credentials:** DB name/user, WP admin user/pass
- **Quick Actions:**
  - **Restart** — restart PHP-FPM và Nginx cho site
  - **Suspend** — tạm dừng site (trả về 503)
  - **Resume** — khôi phục site đang bị suspend
  - **WP Admin** — autologin vào WordPress admin
  - **Visit Site** — mở site trong tab mới
- SSL status và ngày hết hạn

#### Tab Files
Trình quản lý file trực tiếp trên server:
- Duyệt thư mục theo dạng cây
- **Upload** file (kéo thả hoặc chọn file)
- **Download** file
- **Tạo thư mục** mới
- **Đổi tên** file/thư mục
- **Xóa** file/thư mục
- **Chỉnh sửa** file text trực tiếp (editor có syntax highlight)
- **Giải nén** file .zip, .tar.gz
- Xem và thay đổi **permissions** (chmod)

> **Đường dẫn gốc:** `/var/www/{domain}/public/`

#### Tab Staging
Tạo môi trường staging để test trước khi đưa lên production:
- **Create Staging** — clone toàn bộ site sang subdomain `staging.{domain}`
- **Push to Production** — đưa thay đổi từ staging lên production
- **Delete Staging** — xóa staging site
- Xem trạng thái và URL của staging

#### Tab Updates
Quản lý cập nhật WordPress:
- **Check Updates** — kiểm tra bản cập nhật mới cho core/plugins/themes
- Danh sách plugins/themes cần update với version hiện tại và mới nhất
- **Apply Updates** — cập nhật tất cả hoặc từng item
- **Auto-Update Settings:**
  - Bật/tắt tự động cập nhật WordPress core
  - Bật/tắt tự động cập nhật plugins
  - Bật/tắt tự động cập nhật themes
- Lịch sử cập nhật với status và thời gian

#### Tab Database
- **Optimize Database** — chạy OPTIMIZE TABLE để giảm fragmentation
- Thống kê database: số bảng, tổng size

#### Tab Performance
Phân tích hiệu suất site qua Google PageSpeed Insights:
- **Run Test** — chạy phân tích mới
- Điểm **Mobile** và **Desktop** (0-100)
- Chi tiết Lighthouse: Performance, Accessibility, Best Practices, SEO
- Core Web Vitals: LCP, CLS, FID, TTFB
- Lịch sử kết quả đo lường

#### Tab Multisite
Quản lý WordPress Multisite:
- **Enable Multisite** — chuyển sang multisite (subdomain hoặc subfolder)
- **Create Subsite** — tạo site con mới
- **Delete Subsite** — xóa site con
- **Sync Config** — đồng bộ cấu hình Nginx/PHP-FPM cho multisite

#### Tab Images
Tối ưu hóa hình ảnh tự động:
- **Bật/Tắt** image optimization
- **WebP Conversion** — chuyển đổi JPG/PNG sang WebP
- **Nginx WebP** — phục vụ file WebP tự động qua Nginx
- Cấu hình **chất lượng** ảnh (quality %)
- Thống kê: số ảnh đã tối ưu, dung lượng tiết kiệm
- Chạy **optimize ngay** hoặc tự động theo schedule

#### Tab Domains
Quản lý domain cho site:
- **Domain chính** — xem/thay đổi domain chính của site
- **Thêm domain alias** — thêm domain phụ trỏ về cùng site
- Cấu hình **redirect** (301/302) cho từng domain
- Bật/tắt **SSL** per domain
- **Kiểm tra DNS** — xác minh domain đã trỏ đúng IP chưa

#### Tab Redis
Quản lý Redis cache cho site:
- **Bật/Tắt** Redis cache
- Xem thống kê: hit rate, memory used, keyspace
- **Flush cache** — xóa toàn bộ cache Redis của site
- Cấu hình max memory

#### Tab Cron
- Xem WordPress cron jobs đang chạy
- Thống kê cron execution

#### Tab SFTP
Tạo tài khoản SFTP riêng cho site:
- **Tạo SFTP account** — username và password tự tạo
- **Xóa SFTP account**
- Thông tin kết nối: host, port, username, home directory

> Dùng FileZilla, WinSCP hoặc bất kỳ SFTP client nào.

#### Tab Security / WAF
Web Application Firewall per-site:
- **Bật/Tắt WAF** (ModSecurity)
- Chọn **ruleset:** OWASP CRS, Basic, Strict
- Điều chỉnh **Paranoid Level** (1-4)
- **IP Whitelist** — thêm IP không bị filter bởi WAF
- Thêm **custom rules** (ModSecurity syntax)

#### Tab SSL
- Trạng thái SSL hiện tại và ngày hết hạn
- **Issue/Renew SSL** — cấp/gia hạn Let's Encrypt
- **Upload Custom SSL** — dùng certificate của bên thứ 3
- **Force HTTPS** — redirect toàn bộ HTTP → HTTPS

#### Tab WP-CLI
Chạy lệnh WP-CLI trực tiếp trên server:
- Terminal-like interface
- Nhập lệnh WP-CLI (không cần gõ `wp` ở đầu)
- Ví dụ: `plugin list`, `user list`, `option get siteurl`
- Lịch sử các lệnh đã chạy với output

#### Tab Cache
Quản lý Nginx FastCGI Cache:
- **Bật/Tắt** Nginx cache cho site
- Cấu hình **cache time** (ví dụ: 1h, 6h, 24h)
- **Exclusions** — loại trừ URL không cache (VD: `/wp-admin`, `/cart`)
- **Purge Cache** — xóa toàn bộ cache ngay lập tức

#### Tab Import
Import site từ hosting ngoài:
- Nhập **IP server nguồn**, SSH credentials
- Chọn domain cần import
- Theo dõi tiến trình import (rsync files + mysqldump)

### 4.3 Đổi PHP Version

1. Tab **Overview** → nhấn **PHP Version** dropdown
2. Chọn version mới
3. Xác nhận — PHP-FPM pool sẽ được tạo lại với version mới

### 4.4 Xóa Site

1. Trang danh sách Sites → nhấn **...** → **Delete**
2. Nhập domain để xác nhận
3. Nhấn **Delete** — xóa hoàn toàn: files, database, nginx config, PHP-FPM pool, SSL cert, Linux user

---

## 5. Backup & Khôi phục

### 5.1 Tạo Backup thủ công

1. Vào **Backups** → nhấn **+ Create Backup**
2. Chọn **Site**
3. Chọn **Loại backup:**
   - **Full** — toàn bộ files + database (~thời gian lâu hơn)
   - **Database Only** — chỉ backup database (~nhanh hơn)
4. Chọn **Storage** (nếu đã cấu hình remote storage)
5. Nhấn **Create** — theo dõi tiến trình

Hoặc từ trang **Site Detail** → tab **Backup** → **Create Backup**.

### 5.2 Khôi phục Backup

1. Vào **Backups** → tìm backup cần restore
2. Nhấn **Restore**
3. Nhập lại domain để xác nhận (tránh restore nhầm)
4. Nhấn **Confirm Restore**

> **Cảnh báo:** Restore sẽ ghi đè toàn bộ dữ liệu hiện tại của site!

### 5.3 Lên lịch Backup tự động

1. Tab **Backup** của site → nhấn **+ Add Schedule**
2. Cấu hình:
   - **Type:** Full hoặc Database Only
   - **Schedule** (cron expression): ví dụ `0 2 * * *` = 2 giờ sáng mỗi ngày
   - **Retention Days:** số ngày giữ backup (ví dụ: 7, 14, 30)
   - **Storage:** local hoặc remote (S3, R2, Google Drive, Backblaze)
3. Nhấn **Save**

### 5.4 Cấu hình Remote Storage

1. Vào **Settings** → tab **Storage**
2. Nhấn **+ Add Storage**
3. Chọn provider:
   - **AWS S3** — Access Key, Secret Key, Bucket, Region
   - **Cloudflare R2** — Account ID, Access Key, Secret Key, Bucket
   - **Google Drive** — Service Account JSON
   - **Backblaze B2** — Key ID, Application Key, Bucket
4. **Test Connection** để kiểm tra
5. **Save**

### 5.5 Xóa Backup

Nhấn icon **Xóa** trên backup — không thể khôi phục sau khi xóa.

---

## 6. Migration

Migration giúp chuyển site từ server này sang server khác (hoặc đổi domain).

### 6.1 Tạo Migration mới

1. Vào **Migrations** → nhấn **+ New Migration**
2. Chọn **Site** cần migrate
3. Chọn **Server đích**
4. Chọn **Mode:**

   **Keep Domain** — giữ nguyên domain:
   - Site được copy sang server mới
   - Source site bị suspend trong quá trình migrate
   - Tùy chọn **Auto-update DNS** (cần cấu hình DNS provider)

   **Change Domain** — đổi domain mới:
   - Site được copy sang server với domain mới
   - WP search-replace tự động thay toàn bộ URL
   - Source site vẫn chạy trong quá trình migrate

5. Nhấn **Start Migration**

### 6.2 Theo dõi tiến trình

Migration trải qua 5 phase:

| Phase | Mô tả |
|-------|-------|
| **Preparation** | Tạo SSH key, kiểm tra kết nối |
| **Initial Sync** | Copy toàn bộ files và database lần đầu |
| **Delta Sync** | Đồng bộ thay đổi sau initial sync |
| **Switchover** | Cấu hình Nginx/PHP-FPM/SSL trên server đích |
| **Cleanup** | Dọn dẹp và hoàn tất |

Log chi tiết từng bước hiển thị trong panel.

### 6.3 Dọn dẹp Server nguồn

Sau khi migration **completed**:
1. Nhấn **Cleanup Source** để xóa files/DB/configs của site cũ trên server nguồn
2. Giải phóng dung lượng server nguồn

### 6.4 Rollback

Nếu migration thất bại hoặc có vấn đề:
1. Nhấn **Rollback** trên migration
2. Source site được khôi phục, target artifacts bị xóa

---

## 7. Bảo mật (Security)

### 7.1 Malware Scan

1. Vào **Security** → tab **Malware Scans**
2. Nhấn **+ New Scan**
3. Chọn:
   - **Site** cần scan
   - **Scan Type:** Quick (uploads only), Full (toàn bộ), Custom (chỉ định path)
   - **Engine:** WP Signatures, ClamAV, hoặc cả hai
4. Nhấn **Start Scan**

**Kết quả scan:**
- Số file đã quét
- Số mối đe dọa phát hiện
- Chi tiết từng mối đe dọa (file path, loại, mức độ nghiêm trọng)
- Mức độ: Critical 🔴, High 🟠, Medium 🟡, Low 🟢

### 7.2 Lên lịch Scan tự động

1. Tab **Schedules** → **+ Add Schedule**
2. Chọn site, engine, tần suất (6h/12h/24h/48h/72h/168h)
3. **Save**

### 7.3 Cài ClamAV

Nếu server chưa có ClamAV:
1. Trang chi tiết Server → tab **Security** (hoặc qua Health Check)
2. Nhấn **Install ClamAV**
3. Chờ cài đặt hoàn tất (~2-3 phút)

### 7.4 WAF (Web Application Firewall)

Cấu hình per-site tại Site Detail → tab **Security**:
- **OWASP CRS** — bộ rule chuẩn, bảo vệ XSS, SQL injection, LFI...
- **Paranoid Level 1-4** — càng cao càng chặt (có thể false positive)
- **IP Whitelist** — IP được bypass WAF (dùng cho IP của team)

---

## 8. Giám sát (Monitoring)

### 8.1 Uptime Monitoring

1. Vào **Monitoring**
2. Nhấn **+ Add Check**
3. Chọn **Site** và **URL** cần monitor (mặc định homepage)
4. Chọn **Interval:** 60s / 5m / 15m / 30m
5. **Save**

**Trạng thái:**
- 🟢 **Online** — HTTP 2xx/3xx
- 🔴 **Offline** — HTTP 4xx/5xx hoặc timeout
- ⚪ **No data** — chưa có lần check nào

**Lịch sử:** Xem 30 lần check gần nhất với response time (ms).

### 8.2 Alert Rules

Cảnh báo khi tài nguyên server vượt ngưỡng:

1. Trang chi tiết Server → tab **Alerts** → **+ Add Rule**
2. Cấu hình:
   - **Metric:** CPU, RAM, hoặc Disk
   - **Threshold:** ngưỡng % (ví dụ: 85%)
   - **Channel:** Telegram, Slack, hoặc Email
   - **Webhook URL** / **Email** nhận cảnh báo
   - **Cooldown:** thời gian chờ giữa 2 cảnh báo (phút)
3. **Save**

**Telegram setup:**
- Tạo bot qua @BotFather, lấy token
- Lấy Chat ID của group/channel
- Webhook URL: `https://api.telegram.org/bot{TOKEN}/sendMessage`

### 8.3 SSL Expiry Alerts

Panel tự động kiểm tra SSL hàng ngày và gửi cảnh báo khi còn:
- 14 ngày
- 7 ngày
- 3 ngày
- 1 ngày

---

## 9. DNS Management

### 9.1 Thêm DNS Provider

1. Vào **DNS** → **+ Add Provider**
2. Chọn provider: **Cloudflare**, **Route53**
3. Nhập credentials:
   - **Cloudflare:** API Token, Zone ID
   - **Route53:** Access Key, Secret Key, Hosted Zone ID
4. Nhấn **Test Connection**
5. **Save**

### 9.2 Quản lý DNS Records

1. Chọn provider → chọn domain
2. Xem danh sách records hiện tại
3. Nhấn **+ Add Record:**
   - Type: A, AAAA, CNAME, MX, TXT, NS, SRV
   - Name, Value, TTL
   - Cloudflare: toggle **Proxy** (bật/tắt orange cloud)
4. **Edit** hoặc **Delete** record hiện có

### 9.3 Auto DNS Update trong Migration

Khi migrate với mode **Keep Domain** + bật **Update DNS**:
- Panel tự động cập nhật A record của domain trỏ về IP server đích
- Cần đã cấu hình DNS Provider cho domain đó

---

## 10. Teams & Phân quyền

### 10.1 Tạo Team

1. Vào **Teams** → nhấn **+ Create Team**
2. Nhập **tên** và **mô tả** team
3. **Save**

### 10.2 Thêm thành viên

1. Chọn team → tab **Members**
2. Nhấn **+ Add Member**
3. Chọn user từ danh sách
4. Chọn **Role trong team:** Admin hoặc Member
5. **Add**

### 10.3 Gán Server cho Team

1. Chọn team → tab **Servers**
2. Nhấn **+ Assign Server**
3. Chọn server
4. **Assign**

Tất cả thành viên trong team sẽ có quyền truy cập các server được gán.

### 10.4 Phân quyền hệ thống

| Role | Quyền hạn |
|------|-----------|
| **Admin** | Toàn quyền: quản lý users, servers, sites, settings |
| **Manager** | Quản lý servers và sites được assign |
| **Viewer** | Chỉ xem, không thực hiện thao tác |

> Admin được tạo ở lần đăng ký đầu tiên. Các user tiếp theo phải được Admin tạo qua **Users** page.

---

## 11. Quản lý Users

> Chỉ **Admin** mới truy cập được trang này.

### 11.1 Tạo User mới

1. Vào **Users** → nhấn **+ Add User**
2. Điền:
   - **Email** (dùng để đăng nhập)
   - **Tên hiển thị**
   - **Mật khẩu** (phải đủ mạnh)
   - **Role:** Admin, Manager, hoặc Viewer
3. **Create**

### 11.2 Chỉnh sửa User

Nhấn **Edit** trên user:
- Thay đổi tên, email, role
- Đặt lại mật khẩu mới

### 11.3 Gán Server cho User

1. Nhấn **Assign Servers** trên user
2. Chọn các server user được phép truy cập
3. **Save**

> User với role **Admin** tự động có quyền truy cập tất cả servers.

### 11.4 Xóa User

Nhấn **Delete** → xác nhận → user bị xóa khỏi hệ thống.

---

## 12. Cài đặt (Settings)

### 12.1 Bảo mật tài khoản

**Đổi mật khẩu:**
1. Settings → tab **Security**
2. Nhập mật khẩu hiện tại và mật khẩu mới
3. **Save**

**API Keys:**
1. Settings → tab **API Keys**
2. Nhấn **Generate Key** để tạo key mới
3. Sao chép key (chỉ hiện 1 lần)
4. Dùng key trong `Authorization: Bearer {key}` header cho API calls

### 12.2 Thông báo (Notifications)

Cấu hình kênh nhận thông báo toàn hệ thống:

**Email:**
- SMTP Host, Port
- Username, Password
- From address

**Telegram:**
- Bot Token
- Chat ID

**Slack:**
- Webhook URL

### 12.3 Cập nhật Panel (Admin only)

1. Settings → tab **Panel Updates**
2. Nhấn **Check for Updates**
3. Nếu có bản mới: xem release notes → nhấn **Apply Update**
4. Panel sẽ download binary mới, replace và restart tự động

### 12.4 Cập nhật Agent (Admin only)

1. Settings → tab **Agent Updates**
2. **Thêm release mới:**
   - Version (ví dụ: `1.5.0`)
   - Download URL của binary
   - Release notes
3. Tạo **Rollout Plan** để deploy agent mới lên các server
4. Theo dõi trạng thái rollout theo từng server

---

## 13. Activity Log

Ghi lại toàn bộ hoạt động của mọi user trong hệ thống.

### Xem logs

1. Vào **Activity Log**
2. Mỗi entry hiển thị:
   - **Thời gian**
   - **User** thực hiện
   - **Action** (ví dụ: create_site, delete_backup, login...)
   - **Resource** bị ảnh hưởng (server/site)
   - **IP Address**
   - **Chi tiết** thêm

### Lọc logs

- **Theo User** — xem hành động của một user cụ thể
- **Theo Server** — xem hành động trên một server
- **Theo Action** — filter theo loại hành động
- **Theo khoảng thời gian** — từ ngày đến ngày

### Dọn dẹp logs

**Settings → Activity Log:**
- Nhập số ngày retention (ví dụ: 90 ngày)
- Nhấn **Cleanup** — xóa logs cũ hơn X ngày

---

## Phụ lục

### Trạng thái Site

| Status | Ý nghĩa |
|--------|---------|
| `active` | Site đang chạy bình thường |
| `suspended` | Site bị tạm dừng, trả về 503 |
| `creating` | Đang trong quá trình tạo |
| `error` | Có lỗi trong quá trình tạo/thao tác |
| `deleting` | Đang trong quá trình xóa |

### Trạng thái Server

| Status | Ý nghĩa |
|--------|---------|
| `active` | Agent đang kết nối, heartbeat bình thường |
| `provisioning` | Đang cài đặt agent |
| `offline` | Không nhận được heartbeat (>5 phút) |
| `error` | Lỗi trong quá trình provisioning |

### Cron Expression nhanh

| Expression | Ý nghĩa |
|-----------|---------|
| `0 2 * * *` | 2:00 sáng mỗi ngày |
| `0 2 * * 0` | 2:00 sáng mỗi Chủ nhật |
| `0 */6 * * *` | Mỗi 6 tiếng |
| `0 2 1 * *` | 2:00 sáng ngày 1 mỗi tháng |

### Phím tắt trong Panel

| Phím | Chức năng |
|------|-----------|
| `Cmd+Shift+R` | Hard refresh (khi cần load JS mới) |

### Hỗ trợ

Nếu gặp vấn đề không tự xử lý được:
1. Kiểm tra **Activity Log** để xem lỗi chi tiết
2. Kiểm tra **Server → tab Logs** để xem log server
3. Chạy **Health Check** trên server để kiểm tra trạng thái dịch vụ
