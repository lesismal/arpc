## Gin Web Chat

This example re-works the webchat example to work hosted in a [Gin web application](https://github.com/gin-gonic/gin?tab=readme-ov-file#gin-web-framework).

The primary changes are a http web api is exposed `GET /room` returning the identifiers of all users connected to the chat.

```bash
> curl http://localhost:8888/room 
{"users":[0,10000,10001]}
```  