#!/bin/sh
# Prints how many lines the files given hold, all together.
total=0
for file in $@; do
  lines=$(wc -l < $file)
  total=$((total + lines))
done
echo $total
