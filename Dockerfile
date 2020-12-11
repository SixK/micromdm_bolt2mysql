FROM golang:latest as builder

WORKDIR /go/src/github.com/micromdm/micromdm/

ENV CGO_ENABLED=0 \
	GOARCH=amd64 \
	GOOS=linux

COPY . .


RUN git clone --depth 1 https://github.com/SixK/micromdm.git /tmp/micromdm

RUN cp -r /tmp/micromdm/* /go/src/github.com/micromdm/micromdm/
# RUN go run micromdm_bolt2mysql.go
RUN go build micromdm_bolt2mysql.go

CMD ["bash"]
