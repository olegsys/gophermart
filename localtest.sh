#!/usr/local/bin/fish
set DATABASE_CONN_STRING postgres://myuser:mypassword@localhost:5432/mydatabase?sslmode=disable
set -x ACCRUAL_SYSTEM_ADDRESS http://localhost:9090
go build -o cmd/gophermart/gophermart cmd/gophermart/main.go
gophermarttest -test.v -test.run=^TestGophermart\$ \
-gophermart-binary-path=cmd/gophermart/gophermart \
-gophermart-host=localhost \
-gophermart-port=8080 \
-gophermart-database-uri=$DATABASE_CONN_STRING \
-accrual-binary-path=cmd/accrual/accrual_darwin_amd64 \
-accrual-host=localhost \
-accrual-port=9090 \
-accrual-database-uri=$DATABASE_CONN_STRING
rm cmd/gophermart/gophermart
