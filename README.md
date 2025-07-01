# Introduction 

TODO:  
tests for DequeueWithReservation, ConfirmReservation and RequeueExpiredReservations  
tests for /dequeuesafe and /confirm and StartRequeueTask in Server
update swagger for /dequeuesafe and /confirm
requeue cannot run before file is fully loaded


# Init
```
git config --global user.name "Your Name"
git config --global user.email "your.email@example.com"
go mod init github.com/jnsoft/jnq

go get -u github.com/go-chi/chi/v5
go get -u github.com/go-openapi/runtime/middleware
go get -u golang.org/x/exp/mmap
go get -u github.com/google/uuid
go get -u github.com/jnsoft/jngo

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
./.bin/app -key api-key -m

go run ./src/main.go -key api-key -db test.db -v
go run ./src/main.go -key api-key -m


go build -o ./.bin/client ./workerclient/workerclient.go
./.bin/client http://localhost:8080 1 api-key 1 1
```

### Podman build
```
podman build -t jnq .

read -s API_KEY
podman run -d --rm -p 8080:8080 jnq -key $API_KEY

mkdir -p ~/jnq-data
podman run -d --rm -p 8080:8080 -v ~/jnq-data:/app/data jnq -key $API_KEY -db /app/data/test.db
podman run -d --rm -p 8080:8080 -v ~/jnq-data:/app/data jnq -key $API_KEY -m -f /app/data/test

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

curl -X GET "http://localhost:8080/dequeuesafe?channel=1" -H "X-API-Key: api-key"

curl -X POST "http://localhost:8080/confirm" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: api-key" \
  -d '{"reservation_id": "1-1696857600000000"}'
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
