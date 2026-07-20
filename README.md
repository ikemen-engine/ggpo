# [![GGPO-Go LOGO](./ggpo_go_logo.png)](https://github.com/ikemen-engine/ggpo)
# GGPO-Go - GGPO Port Into Go
GGPO-Go is a port of the GGPO rollback netcode library into Go. Currently unfinished. 

## Usage 
General usage would be best explained by looking at the code in the example folder.

## Running the UDP backend example

follow the following steps: 

- Clone the repository. 
- enter:
    `go run . <local_port> <num_players>  <local|remote_ip:port> <local|remote_ip:port> <current_player>`

An example usage would be to open one command line input with the following command 
`go run ./example/ 7000 2 local 127.0.0.1:7001 1`
and another with this command 
`go run ./example/ 7001 2 127.0.0.1:7000 local 2`

If you want to have a spectator, make sure the spectator connect to the host FIRST before the host connects with player 2

player 1 (the "host" — note the spectator address as the last arg)
`go run ./example/ 7000 2 local 127.0.0.1:7001 1 127.0.0.1:7100`

spectator, listening on 7100, watching player 1
`go run ./example/ 7100 2 spectate 127.0.0.1:7000`

player 2
`go run ./example/ 7001 2 127.0.0.1:7000 local 2`

## Running the WebRTC backend example

If you want to run the webrtc demo, then do

run the signaling server:
``go run github.com/ikemen-engine/ggpo/cmd/signaling -addr :3000``

If you want to run the WASM build, do
``go run ./example/webrtc/serve``

and then in two different browser tabs, go to
tab 1: http://localhost:8080/?host=1&lobby=test&signaling=http://localhost:3000
tab 2: http://localhost:8080/?lobby=test&signaling=http://localhost:3000

where http://localhost:3000 is the address to your signaling server

They should connect and you can then move around with arrow keys.

WebRTC should work natively as well, just run

`go run ./example/webrtc -host -lobby test`

and then

`go run ./example/webrtc -lobby test` 

in a separate terminal to connect.

## Test WebRTC backend over network
running `scripts\package.sh` will give you a ready to use package to run on a cloud linux server.
Just upload it onto your linux server, and then do on your linux server

`tar -xzf webrtcserver-linux-amd64.tar.gz`

and then

`./webrtcserver -addr 127.0.0.1:8080 -dir public`

and then in two different browser tabs, go to
tab 1: http://your.web.server/?host=1&lobby=test
tab 2: http://your.web.server/?lobby=test
