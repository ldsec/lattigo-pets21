#!/bin/bash

if [ "$#" -eq 0 ]; then
    echo "[he|mhe] [n_parties] [n_triples] [filename]";
	exit;
fi

localNet=0
docker network inspect mpc-net &> /dev/null
if [ $? -ne 0 ]; then
   echo "creating local network"
  localNet=1 
  docker network create mpc-net > /dev/null
fi

for (( c=1; c<=$2-1; c++))
do

  if [ -z "${4+x}" ]; then file=/dev/null; else file=$4_p$c.txt; fi

  docker run --name party-$c --net mpc-net --rm mhe-exps tpl $1 $c $2 $3 &> $file &

done


if [ -z "${4+x}" ]; then file=/dev/null; else file=$4_p0.txt; fi

docker run --name party-0 --net mpc-net --rm mhe-exps tpl $1 0 $2 $3 2>&1 | tee -a ${file} 

if [ $localNet -ne 0 ]; then
  echo "removing local network"
  sleep 3
  docker network rm mpc-net > /dev/null;
fi
