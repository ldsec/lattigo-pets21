# Multiparty Homomorphic Encyption from Ring-Learning-with-Errors: Artifacts

This repository regroups the software artifacts for the article _Multiparty Homomorphic Encryption from Ring-Learning-with-Errors_ [1] presented at the 21st Privacy Enhancing Technologies Symposium (PETS'21).

## Artifacts list

The following sowftare items are artifacts of the article:

| Artifact                                    | Description                                                                                                       |
| :------------------------------------------ | ----------------------------------------------------------------------------------------------------------------- |
|  `github.com/ldsec/lattigo/v2/dbfv`         | the [Lattigo Go package](https://github.com/ldsec/lattigo/dbfv) implementing the multiparty BFV scheme.           |                                         |
|  `github.com/ldsec/lattigo-pets21/apps`     | a Go module importing the `github.com/ldsec/lattigo/v2/dbfv` Lattigo sub-package and implementing the experiments | 
|  `github.com/ldsec/lattigo-pets21/apps/pir` | a Go application that implements the PIR experiment                                                               | 
|  `github.com/ldsec/lattigo-pets21/apps/psi` | a Go application that implements the PSI experiment                                                               |
|  `github.com/ldsec/lattigo-pets21/apps/tpl` | a Go application that implements the Beaver-triples-generation experiment                                         |


## Building

On a machine running Docker, running
```
make
```
will build a `mhe-exps` docker image for which the three experiment apps' binaries are in the `PATH`.

## Running

### Multiparty-Input-Selection (PIR) and Element-Wise-Vector-Product (PSI) experiments

The PIR and PSI experiment are local and running the client and server within the same process:
```
docker run --rm mhe-exps pir    // runs the PIR experiment
docker run --rm mhe-exps psi    // runs the PSI experiment
```

### Multiplication-Triple-Generation experiment

The Beaver-triples-generation experiment runs every party in its own process, by running several instances of the `mhe-exps` image.
The `run-parties.sh` script automates the process of starting the experiment for a given generation technique, number of parties and number of required triples. 
```
./run-parties.sh [he|mhe] [n_parties] [n_triples] [filename]
```
The `stdout` of party 0 is always redirected to the host `stdout`. The script also accepts a filename as an option final argument.
If provided, it saves the `stdout` of each party to a file `[filename]_p[party id].txt`. 

Finally, the `run-exp-tpl.sh` automates the process of running the Beaver-triples-generation experiment for both the `he` and `mhe` generation techniques, for 2 to 16 parties. The `stdout` of each party in each experiment is redirected to a file in the `output` directory.

## Cleaning up

There are two make targets for the clean-up tasks: 

`make clean`: deletes the `output` directory

`make clean-all`: removes the `mhe-exps` images from the Docker host

## References

[1] Christian Mouchet, Juan Troncoso-Pastoriza, Jean-Philippe Bossuat, Jean-Pierre Hubaux. 2021. Multiparty Homomorphic Encryption from Ring-Learning-with-Errors. To be presented at PETS'21.
