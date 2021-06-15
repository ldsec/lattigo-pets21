# Multiparty Homomorphic Encyption from Ring-Learning-with-Errors: Artifacts

This repository hosts the software artifacts for the article _Multiparty Homomorphic Encryption from Ring-Learning-with-Errors_ [1] presented at the 21st Privacy Enhancing Technologies Symposium (PETS'21).

## Artifacts list

The following sowftare items are artifacts of the article:

| Artifact                               | Description                                                                                                        |
| :------------------------------------- | -----------------------------------------------------------------------------------------------------------------  |
|  `lattigo/v2/dbfv`                     | the [Lattigo Go package](https://github.com/ldsec/lattigo/tree/master/dbfv) implementing the multiparty BFV scheme.|
|  `lattigo-pets21/apps`                 | a Go module importing the `github.com/ldsec/lattigo/v2/dbfv` Lattigo sub-package and implementing the experiments  |
|  `lattigo-pets21/apps/pir`             | a Go application that implements the PIR experiment                                                                |
|  `lattigo-pets21/apps/psi`             | a Go application that implements the PSI experiment                                                                |
|  `lattigo-pets21/apps/tpl`             | a Go application that implements the Beaver-triples-generation experiment                                          |


The `lattigo/v2/dbfv` package is integrated in the official Lattigo repository at [https://github.com/ldsec/lattigo](https://github.com/ldsec/lattigo).
The `lattigo-pets21/apps` module is in this repository and imports the latest version of the `lattigo/v2/dbfv` package via a Go module dependency.
This repository includes a `Makefile`, a `Dockerfile` and several scripts that automate building and running our code. 

## Building

From a clone of this repository on a machine running Docker, running
```
make
```
will build a `mhe-exps` docker image for which the three experiment apps' binaries are in the `PATH`.

## Running

### Multiparty-Input-Selection (PIR) and Element-Wise-Vector-Product (PSI) experiments

The PIR and PSI experiments are local and running the client and server within the same process. Both programs take the number of input-parties and the number of goroutines (threads) for the circuit-evaluation by the cloud:
```
docker run --rm mhe-exps [psi|pir] [#parties] [#goroutines] 
```

Exemples:
```
docker run --rm mhe-exps pir        # runs the PIR experiment over 8 parties with a single-threaded cloud evaluation

docker run --rm mhe-exps psi 16     # runs the PSI experiment over 16 parties with a single-threaded cloud evaluation

docker run --rm mhe-exps psi 16 8   # runs the PSI experiment over 16 parties with cloud evaluation using 8 threads
```


### Multiplication-Triple-Generation experiment

The Beaver-triples-generation experiment runs every party in its own process, by running several instances of the `mhe-exps` image within docker network named `mpc-net`.
The `run-tpl-parties.sh` script automates the process of starting the experiment for a given generation technique and number of parties. 
```
./run-tpl-parties.sh [he|mhe] [#parties] [filename]
```
The `stdout` of party 0 is redirected to the host `stdout`. The script also accepts a filename as an option final argument.
If provided, it saves the `stdout` of each party to a file `[filename]_p[party id].txt`. 

Finally, the `run-tpl-exp.sh` automates the process of running the Beaver-triples-generation experiment for both the `he` and `mhe` generation techniques, for 2 to 8 parties. The `stdout` of each party in each experiment is redirected to a file in the `output` directory.

*Note*: Dockerization of the experiment seems to be a little less stable than our initial setting, especially when run on less powerful systems. Some isolated experiments might fail because docker cannot bring the container up fast enough and some tcp connections are sometime reset. These experiments can be restarted indivitually by using the `run-tpl-parties.sh` script with the corresponding arguments.

## Cleaning up

There are two make targets for the clean-up tasks: 

`make clean-output`: deletes the `output` directory.

`make clean-docker`: removes the `mhe-exps` images from the Docker host and the `mpc-net` docker network.

`make clean-all`: performes the `clean-output` and `clean-docker` targets.

## References

[1] Christian Mouchet, Juan Troncoso-Pastoriza, Jean-Philippe Bossuat, Jean-Pierre Hubaux. 2021. Multiparty Homomorphic Encryption from Ring-Learning-with-Errors. To be presented at PETS'21.
