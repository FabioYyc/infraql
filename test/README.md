
# Testing approach

## Contrived local integration testing

Offline invocations of `infraql` are assessed against expected responses, through the functionality of [/test/python/main.py](/test/python/main.py).  These are backed by data stored in cache files in [/test/.infraql/ttl_cache](/test/.infraql/ttl_cache).  These scripts are run during the build process:
  - locally through cmake as per [/README.md#build](/README.md#build)
  - in github actions based CICD as per [/.github/workflows/go.yml](/.github/workflows/go.yml).

## Unit tests using standard golang approaches

Proliferation is a fair way behind development.

These are also run inside build processes: local and remote.

## E2E integration tests

TBA.


## Sundry opinions about testing in golang

  - [Simple approach and dot import.](https://medium.com/@benbjohnson/structuring-tests-in-go-46ddee7a25c)
  - [Making use of containers, make and docker-compose for integration testing.](https://blog.gojekengineering.com/golang-integration-testing-made-easy-a834e754fa4c)
  - [HTTP client testing.](http://hassansin.github.io/Unit-Testing-http-client-in-Go)
  - [Mocking HTTPS in unit tests.](https://stackoverflow.com/questions/27880930/mocking-https-responses-in-go)