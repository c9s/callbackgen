#!/bin/bash
rm ./example/user/*_callbacks.go
GO111MODULE=off go run . -type User -generateRemove -lockField mu -interface ./example/user

