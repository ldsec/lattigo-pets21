#!/bin/bash

NBEAVER=8192


protos="he mhe"

localNet=0
docker network inspect mpc-net &> /dev/null
if [ $? -ne 0 ]; then
   echo "creating local network"
  localNet=1 
  docker network create mpc-net > /dev/null
fi

mkdir -p output

for proto in $protos
do
  for (( c=2; c<=16; c=c+1 ))
  do
    for (( r=1; r<=2; r++ ))
    do
        ./run-tpl-parties.sh $proto $c $NBEAVER output/exp_tpl_${proto}_${c}_${r}
        sleep 5;
    done
  done
done

if [ $localNet -ne 0 ]; then
  echo "removing local network"
  sleep 3
  docker network rm mpc-net > /dev/null;
fi
