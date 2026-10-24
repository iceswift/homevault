package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"

	"homevault/internal/drive"
)

func runDrive(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return errors.New("usage: drive-setup NEW_DIRECTORY --client-json FILE | drive-auth DIRECTORY --confirm | drive-status DIRECTORY [--online]")
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	switch args[0] {
	case "drive-setup":
		client := fs.String("client-json", "", "absolute private downloaded Google Desktop OAuth JSON")
		remote := fs.String("remote", "gdrive", "safe rclone remote name")
		project := fs.String("project-id", "", "optional expected Google Cloud project ID")
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return errors.New("unexpected drive-setup argument")
		}
		if err := drive.Setup(args[1], *client, *remote, *project); err != nil {
			return err
		}
		fmt.Println("Private Drive connection prepared with drive.file scope. No authorization, network request, upload or existing config change.")
		fmt.Println("Next: drive-auth DIRECTORY --confirm, then drive-status DIRECTORY --online. Keep credentials outside Git and backup sources.")
		return nil
	case "drive-auth":
		port := fs.Int("port", 53682, "loopback callback port; 0 selects a random local port")
		confirm := fs.Bool("confirm", false, "explicitly start Google browser consent")
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		if fs.NArg() != 0 || !*confirm {
			return errors.New("usage: drive-auth DIRECTORY [--port 53682] --confirm")
		}
		fmt.Println("Google Drive desktop authorization: requested scope drive.file. Select the intended Google account in your browser. No account password is entered here.")
		err := drive.Authorize(ctx, args[1], *port, func(link string) error {
			parsed, _ := url.Parse(link)
			redirect, _ := url.Parse(parsed.Query().Get("redirect_uri"))
			port := redirect.Port()
			fmt.Printf("For a remote SSH server, open a second terminal on your computer: ssh -N -L 127.0.0.1:%s:127.0.0.1:%s USER@SERVER\n", port, port)
			fmt.Println("Open this Google authorization URL in your computer's browser (do not paste it into chat/issues):")
			fmt.Println(link)
			fmt.Println("Waiting up to 10 minutes. The callback listener accepts only 127.0.0.1 requests.")
			return nil
		})
		if err != nil {
			return err
		}
		fmt.Println("Authorization saved privately. rclone will manage refresh; browser consent success does not prove backup/restore. Next: drive-status DIRECTORY --online.")
		return nil
	case "drive-status":
		online := fs.Bool("online", false, "explicitly check read-only Drive reachability; may refresh token")
		binary := fs.String("rclone-binary", "rclone", "rclone executable for optional online check")
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return errors.New("unexpected drive-status argument")
		}
		status, err := drive.Status(ctx, args[1], *online, *binary)
		if status.Remote != "" {
			if encodeErr := json.NewEncoder(os.Stdout).Encode(status); encodeErr != nil {
				return encodeErr
			}
		}
		return err
	}
	return errors.New("unknown Drive setup command")
}
