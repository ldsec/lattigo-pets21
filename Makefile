docker:
	docker build -t mhe-exps .

clean-output:
	rm -rf output

clean-docker:
	docker image rm mhe-exps
	docker network rm mpc-net 

clean-all: clean-output clean-docker

