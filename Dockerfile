# The base go-image
FROM golang:1.16-alpine
 
# Create a directory for the app
RUN mkdir /apps

 
# Copy all files from the current directory to the app directory
COPY ./apps /apps
 
# Set working directory
WORKDIR /apps
 RUN mkdir bin
# Run command as described:
# go build will build an executable file named server in the current directory
RUN go build -o bin/psi ./psi
RUN go build -o bin/pir ./pir
RUN go build -o bin/tpl ./tpl

ENV PATH="/apps/bin:${PATH}"

CMD [ "/apps/bin/psi" ]