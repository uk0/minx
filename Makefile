
build:
	@echo "Building.."
	go build -o minx  .
	./minx -h
	@echo "Build done ...."
