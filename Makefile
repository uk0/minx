
build:
	@echo "Building.."
	go build -o minx  .
	./minx -h
	@echo "Build done ...."

install:
	@echo "Installing.."
	cp minx /usr/local/bin/minx
	chmod +x /usr/local/bin/minx
	@echo "Install done ...."