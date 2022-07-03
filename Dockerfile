FROM golang:1.17.3
RUN curl -s https://install.speedtest.net/app/cli/install.deb.sh | bash
RUN apt-get install speedtest
RUN apt-get install traceroute
COPY speedtest-cli.json /root/.config/ookla/speedtest-cli.json
RUN mkdir /app
ADD . /app
WORKDIR /app
RUN go build -o main .
CMD ["/app/main"]
