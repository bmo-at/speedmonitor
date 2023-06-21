FROM golang:latest
RUN curl -s https://packagecloud.io/install/repositories/ookla/speedtest-cli/script.deb.sh | bash
RUN apt-get install speedtest
RUN apt-get install traceroute
RUN apt-get install iputils-ping -y
RUN apt-get install jc -y
# COPY speedtest-cli.json /root/.config/ookla/speedtest-cli.json
RUN mkdir /app
ADD . /app
WORKDIR /app
RUN go build -o main .
CMD ["/app/main"]
