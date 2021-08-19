SHELL := /bin/bash

test: lint vet build

test_win: lint vet build_win

lint:
	golint ./...

vet:
	go vet ./...

build: clean
	go build -v -o ./bin/publisher ./cmd/publisher
	go build -v -o ./bin/subscriber ./cmd/subscriber

build_win: clean_win
	go build -v -o ./bin/publisher.exe ./cmd/publisher
	go build -v -o ./bin/subscriber.exe ./cmd/subscriber

clean:
	rm -rf ./bin ./tmp

clean_win:
	if exist bin ( rmdir /S /Q bin )
	if exist tmp ( rmdir /S /Q tmp )

exec_pub:
	./bin/publisher ./config/publisher.cfg

exec_sub:
	./bin/subscriber ./config/subscriber.cfg

exec_win: build_win
	CMD /C start CMD /K .\bin\publisher.exe .\config\publisher.cfg
	timeout 1
	CMD /C start CMD /K .\bin\subscriber.exe .\config\subscriber.cfg

exec_win_cli: build_win
	CMD /C start CMD /K .\bin\subscriber.exe .\config\subscriber.cfg

multi_exec_win: build_win
	CMD /C start CMD /K .\bin\publisher.exe .\config\publisher.cfg
	timeout 1
	CMD /C start CMD /K .\bin\subscriber.exe .\config\subscriber_multi1.cfg
	timeout 1
	CMD /C start CMD /K .\bin\subscriber.exe .\config\subscriber_multi2.cfg
	timeout 1
	CMD /C start CMD /K .\bin\subscriber.exe .\config\subscriber_multi3.cfg

dynamic_exec_win: build_win
	CMD /C start CMD /K .\bin\publisher.exe .\config\publisher_dynamic.cfg
	timeout 1
	CMD /C start CMD /K .\bin\subscriber.exe .\config\subscriber_multi1.cfg
	timeout 1
	CMD /C start CMD /K .\bin\subscriber.exe .\config\subscriber_multi2.cfg
	timeout 1
	CMD /C start CMD /K .\bin\subscriber.exe .\config\subscriber_multi3.cfg

dynamic_cli_win:
	CMD /C start CMD /K .\bin\subscriber.exe .\config\subscriber_multi1.cfg
	timeout 1
	CMD /C start CMD /K .\bin\subscriber.exe .\config\subscriber_multi2.cfg
	timeout 1
	CMD /C start CMD /K .\bin\subscriber.exe .\config\subscriber_multi3.cfg

dynamic_acceptor_win: build_win
	CMD /C start CMD /K .\bin\publisher.exe .\config\publisher_dynamic.cfg

dynamic_store_win: build_win
	CMD /C start CMD /K .\bin\publisher.exe .\config\publisher_dynamic_store.cfg
	timeout 1
	CMD /C start CMD /K .\bin\subscriber.exe .\config\subscriber_dynamic_store1.cfg
	timeout 1
	CMD /C start CMD /K .\bin\subscriber.exe .\config\subscriber_dynamic_store2.cfg

dynamic_store_cli_win:
	CMD /C start CMD /K .\bin\subscriber.exe .\config\subscriber_dynamic_store1.cfg

dynamic_store_win_store_msg:
	CMD /C start CMD /K .\bin\publisher.exe .\config\publisher_dynamic_store.cfg StoreMessage
	timeout 1
	CMD /C start CMD /K .\bin\subscriber.exe .\config\subscriber_dynamic_store1.cfg
	timeout 1
	CMD /C start CMD /K .\bin\subscriber.exe .\config\subscriber_dynamic_store2.cfg

