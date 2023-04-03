#!/bin/bash

if ! which inotifywait >/dev/null; then
  echo "Error: inotifywait required (install inotify-tools)."
  exit 1
fi

trap "trap - SIGTERM && kill -- -$$" SIGINT SIGTERM EXIT

mkdir -p bin

while true; do
  while ! go build -o bin/server ./cmd/server; do
    inotifywait -e close_write,moved_to,create -q lib/ cmd/server/ traces/
    clear
  done

  bin/server &
  server_pid=$!

  inotifywait -e close_write,moved_to,create -q lib/ cmd/server/ traces/
  kill $server_pid
  wait $server_pid
  echo Restarting
done