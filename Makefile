SRCFILES := $(shell find . -iname *.go -type f -print)
PYFILES := $(shell find . -iname *.py -type f -print)
PREFIX := /usr/local
VERSION := 0.1.0

.PHONY: all
all: mskpipe python

mskpipe: $(SRCFILES) Makefile go/go.sum
	@printf 'GO\t%s\n' '$@'
	@cd go && go build -o ../mskpipe ./cmd/mskpipe

.PHONY: python
python: py/dist/misirka-$(VERSION)-py3-none-any.whl

py/dist/misirka-$(VERSION)-py3-none-any.whl: $(PYFILES) Makefile
	@printf 'PYTHON\t%s\n' '$@'
	@cd py && python3 setup.py bdist_wheel

.PHONY: install
install: all
	install -Dm755 mskpipe $(DESTDIR)$(PREFIX)/bin/mskpipe
	pip3 install py/dist/misirka-$(VERSION)-py3-none-any.whl


.PHONY: clean
clean:
	@rm -rf -- mskpipe
	@rm -rf -- py/build py/dist
