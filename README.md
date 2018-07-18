Sharer
======

### A personal and configurable file/link sharer

## Deploy
This app uses [Google App Engine](https://cloud.google.com/appengine/) as the underlying system. The Google App Engine free tier should suffice for personal usage.
1. Install the gcloud sdk
    - https://cloud.google.com/sdk/install
2. Login to gcloud
    - `gcloud auth login`
3. Update your `app.yaml` to reflect your configuration. Go get packages
    - `go get -u github.com/gorilla/mux google.golang.org/appengine google.golang.org/appengine/blobstore google.golang.org/appengine/datastore cloud.google.com/go/storage gopkg.in/h2non/filetype.v1 github.com/dsoprea/goappenginesessioncascade`
4. Run the gcloud app deploy on the project for Sharer
    - `gcloud app deploy app.yaml`
    
## Usage
I personally live in the terminal, so it has been made to be used terminal first. Maybe one day I'll add a Web UI, but this works just fine.
Here are the functions that I use in ZSH. Your mileage may vary:

These snippets have been adapted from ones provided by [Dutchcoders](https://dutchcoders.io) for [transfer.sh](https://transfer.sh):
```bash
share() {
    # check arguments
    if [ $# -eq 0 ];
    then
        echo "No arguments specified. Usage:\necho share /tmp/test.md 10m 10 encrypt #(file duration clicks encrypt)\ncat /tmp/test.md | share test.md 10m 10 encrypt #(filename duration clicks encrypt)"
        return 1
    fi

    # get temporarily filename, output is written to this file show progress can be showed
    tmpfile=$( mktemp -t transferXXX )

    # upload stdin or file
    file=$1
    expiresin=$2
    expireclicks=$3
    encrypt=$4
    host="https://HOSTNAME"
    authorization="AUTH_TOKEN"

    if tty -s;
    then
        basefile=$(basename "$file" | sed -e 's/[^a-zA-Z0-9._-]/-/g')

        if [ ! -e $file ];
        then
            echo "File $file doesn't exists."
            return 1
        fi

        if [ -d $file ];
        then
            # zip directory and transfer
            zipfile=$( mktemp -t transferXXX.zip )
            cd $(dirname $file) && zip -r -q - $(basename $file) > $zipfile
            basefile="$basefile.zip"

            if [ ! -z "$encrypt" ];
            then
                gpg --no-symkey-cache --cipher-algo aes256 -c $zipfile
                originalfile=$zipfile
                zipfile="$zipfile.gpg"
                basefile="$basefile.gpg"
            fi

            curl -H "X-Authorization: $authorization" --progress-bar --upload-file "$zipfile" "$host/api/upload/$basefile?s=1&time=$expiresin&clicks=$expireclicks" >> $tmpfile
            rm -f $zipfile

            if [ ! -z "$encrypt" ];
            then
                rm -f $originalfile
            fi
        else
            if [ ! -z "$encrypt" ];
            then
                gpg --no-symkey-cache --cipher-algo aes256 -c $file
                file="$file.gpg"
                basefile="$basefile.gpg"
            fi

            # transfer file
            curl -H "X-Authorization: $authorization" --progress-bar --upload-file "$file" "$host/api/upload/$basefile?s=1&time=$expiresin&clicks=$expireclicks" >> $tmpfile
            if [ ! -z "$encrypt" ];
            then
                rm -f $file
            fi
        fi
    else
        # transfer pipe
        curl -H "X-Authorization: $authorization" --progress-bar --upload-file "-" "$host/api/upload/$file?s=1&time=$expiresin&clicks=$expireclicks" >> $tmpfile
    fi

    # cat output link
    cat $tmpfile

    # cleanup
    rm -f $tmpfile
}
```

```bash
linkshare() {
    link=$1
    expiresin=$2
    expireclicks=$3
    host="https://HOSTNAME"
    authorization="AUTH_TOKEN"

    # check arguments
    if [ $# -eq 0 ];
    then
        echo "No arguments specified. Usage:\necho linkshare https://google.com 10m 10 #(clicks)"
        return 1
    fi

    if tty -s;
    then
        curl -H "X-Authorization: $authorization" -X "POST" "$host/api/shorten?s=1&url=$link&time=$expiresin&clicks=$expireclicks"
    fi
}
```

Update your own hostname (`s/HOSTNAME/YOUR_HOST_HERE/g`) and auth key `s/AUTH_TOKEN/YOUR_AUTH_KEY/g`

Example usages are as follows:

```
# share <file> <duration> <click count>
share superman.jpg 10m 2
```

```
# linkshare <url> <duration> <click count>
linkshare https://google.com 10m 2
```