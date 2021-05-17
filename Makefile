docker:
	docker build -t mhe-exps .

clean:
	rm -rf output

clean-all:
	docker image rm mhe-exps
