HƯỚNG DẪN CÀI ĐẶT TRÊN MÁY MỚI
===============================

Yêu cầu:
- Máy tính đã cài đặt Docker Desktop và Docker Compose.

Các bước thực hiện:
1. Giải nén file `gpm-release.zip` ra một thư mục.
2. Mở Terminal (PowerShell hoặc CMD) tại thư mục vừa giải nén.
3. Chạy lệnh sau để khởi động:
   
   docker-compose up -d --build

4. Xong! Dịch vụ đã chạy.
   - API: http://localhost:8080/can-post/{profile}
   - Dữ liệu sẽ được lưu vào file `profiles.json` và `history.txt` ngay trong thư mục này.

Ghi chú:
- Để dừng server: chạy lệnh `docker-compose down`
- Để xem log: chạy lệnh `docker-compose logs -f`
