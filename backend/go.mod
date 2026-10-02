module github.com/adb-lanlu/prism-oss

go 1.25.0

require (
	github.com/Yeah114/g79client v0.0.0
	github.com/Yeah114/unmcpk v0.0.0
	github.com/google/uuid v1.6.0
	github.com/yuin/gopher-lua v1.1.2
	golang.org/x/crypto v0.53.0
	golang.org/x/image v0.45.0
	golang.org/x/net v0.56.0
	modernc.org/sqlite v1.52.0
)

require (
	github.com/database64128/chacha8-go v0.0.0-20250815115417-e0f2726d8bd0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.47.0 // indirect
	modernc.org/libc v1.72.3 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace github.com/Yeah114/g79client => ./third_party/g79client

replace github.com/Yeah114/unmcpk => ./third_party/unmcpk
