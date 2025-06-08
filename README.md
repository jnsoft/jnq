# Introduction 
TODO: Give a short introduction of your project. Let this section explain the objectives or the motivation behind this project. 

# Init
```
git config --global user.name "Your Name"
git config --global user.email "your.email@example.com"
go mod init github.com/jnsoft/jnq

go get -u github.com/go-chi/chi/v5
go get -u github.com/go-openapi/runtime/middleware

# debug:
dlv debug src/main.go
```

# Build and Test
```
go test -v ./...
go test -v -race ./src/pqueue/ # identify race conditions
go test -v ./src/mempqueue

go run src/generateswagger/generateswagger.go
go build -o .bin/app ./src/main.go
./.bin/app -db test.db -table QueueItems -n 10

go run ./src/main.go -key api-key -n 3 -db test.db -v
go run ./src/main.go -db test.db -table QueueItems -n 10

go build -o ./.bin/client ./workerclient/workerclient.go
chmod +x ./.bin/client
./.bin/client http://localhost:8080 5 api-key 3 2
```

### Podman build
```
podman build -t jnq .

read -s API_KEY

podman run -d --rm -p 8080:8080 jnq -key $API_KEY

curl -X 'POST' \
  "http://localhost:8080/reset" \
  -H "accept: text/plain" \
  -H "X-API-Key: $API_KEY" \
  -d ''

curl -X 'POST' \
  "http://localhost:8080/enqueue?prio=1.5&channel=2&notbefore=2025-04-10T19:37:53Z" \
  -H "accept: text/plain" \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '"this is a message"'

curl -X 'POST' \
  "http://localhost:8080/enqueue?channel=2" \
  -H "accept: text/plain" \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "key" : "value",
    "bool": true,
    "int": 5
}'

curl -X 'GET' \
  "http://localhost:8080/dequeue?channel=2" \
  -H "accept: application/json" \
  -H "X-API-Key: $API_KEY"
```

```
# Enqueue an item
curl -X POST -d '{"value": 42}' http://localhost:8080/enqueue

# Enqueue an item with a notbefore timestamp
curl -X POST "http://localhost:8080/enqueue?notbefore=2025-04-10T19:37:53Z" \
     -H "Content-Type: application/json" \
     -d '{"value": 42}'

# Dequeue an item
curl http://localhost:8080/dequeue

curl -X POST "http://localhost:8080/enqueue" \
     -H "Content-Type: application/json" \
     -d '{
           "value": "{\"name\": \"example\", \"details\": {\"age\": 30, \"hobbies\": [\"reading\", \"coding\"]}}",
           "priority": 1.0,
           "channel": 1
         }'
```

### Misc
```
chmod 600 ~/.netrc

CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o jnq ./src/main.go
```

# Contribute
