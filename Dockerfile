FROM golang:1.20.1

RUN apt-get install traceroute
RUN apt-get install iputils-ping -y

RUN mkdir /app
ADD . /app
WORKDIR /app

RUN go build -o main .
CMD ["/app/main"]
