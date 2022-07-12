# x321

запускаем сервак с указанием куда сохранять файлы

`go run main.go http://127.0.0.1:8086/files/`

запускаем поток на сервер, копия которого сохранится в файл

`ffmpeg -i http://playerservices.streamtheworld.com/api/livestream-redirect/KINK.mp3 -c copy -f rtsp rtsp://127.0.0.1:8554/123`
