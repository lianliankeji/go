#!/bin/bash

declare -A appDirScritpMap=(
    ["./KuaiDian1.0"]="kd.js"
    ["./mg"]="mg.js"
    ["./Mogao"]="mogao.js"
    ["./MogaoTest"]="mogaoTest.js"
    ["./Retail"]="retail.js"
    ["./RetailTest"]="retailTest.js"
    ["./Supervise"]="spvs.js"
    ["./SuperviseTest"]="spvsTest.js"
)



for dir in ${!appDirScritpMap[@]}; do 
    echo ">>>>>> forever start  ${appDirScritpMap[$dir]}"
    (cd "$dir" && forever start  ${appDirScritpMap[$dir]})
done
