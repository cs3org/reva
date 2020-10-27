/*
 * MIT License
 *
 * Copyright (c) 2020 Daniel Mueller
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package main

import (
	"fmt"
	"log"

	"github.com/cs3org/reva/pkg/sdk"
	"github.com/cs3org/reva/pkg/sdk/action"
)

func runActions(session *sdk.Session) {
	// Try creating a directory
	{
		act := action.MustNewFileOperationsAction(session)
		if err := act.MakePath("/home/subdir/subsub"); err == nil {
			log.Println("Created path /home/subdir/subsub")
		} else {
			log.Println("Could not create path /home/subdir/subsub")
		}
		fmt.Println()
	}

	// Try deleting a directory
	{
		act := action.MustNewFileOperationsAction(session)
		if err := act.Remove("/home/subdir/subsub"); err == nil {
			log.Println("Removed path /home/subdir/subsub")
		} else {
			log.Println("Could not remove path /home/subdir/subsub")
		}
		fmt.Println()
	}

	// Try uploading
	{
		act := action.MustNewUploadAction(session)
		act.EnableTUS = true
		if info, err := act.UploadBytes([]byte("HELLO WORLD!\n"), "/home/subdir/tests.txt"); err == nil {
			log.Printf("Uploaded file: %s [%db] -- %s", info.Path, info.Size, info.Type)
		} else {
			log.Printf("Can't upload file: %v", err)
		}
		fmt.Println()
	}

	// Try moving
	{
		act := action.MustNewFileOperationsAction(session)
		if err := act.MoveTo("/home/subdir/tests.txt", "/home/sub2"); err == nil {
			log.Println("Moved tests.txt around")
		} else {
			log.Println("Could not move tests.txt around")
		}
		fmt.Println()
	}

	// Try listing and downloading
	{
		act := action.MustNewEnumFilesAction(session)
		if files, err := act.ListFiles("/home", true); err == nil {
			for _, info := range files {
				log.Printf("%s [%db] -- %s", info.Path, info.Size, info.Type)

				// Download the file
				actDl := action.MustNewDownloadAction(session)
				if data, err := actDl.Download(info); err == nil {
					log.Printf("Downloaded %d bytes for '%v'", len(data), info.Path)
				} else {
					log.Printf("Unable to download data for '%v': %v", info.Path, err)
				}

				log.Println("---")
			}
		} else {
			log.Printf("Can't list files: %v", err)
		}
		fmt.Println()
	}

	// Try accessing some files and directories
	{
		act := action.MustNewFileOperationsAction(session)
		if act.FileExists("/home/blargh.txt") {
			log.Println("File '/home/blargh.txt' found")
		} else {
			log.Println("File '/home/blargh.txt' NOT found")
		}

		if act.DirExists("/home") {
			log.Println("Directory '/home' found")
		} else {
			log.Println("Directory '/home' NOT found")
		}
		fmt.Println()
	}
}

func main() {
	session := sdk.MustNewSession()
	if err := session.Initiate("sciencemesh-test.uni-muenster.de:9600", false); err != nil {
		log.Fatalf("Can't initiate Reva session: %v", err)
	}

	if methods, err := session.GetLoginMethods(); err == nil {
		fmt.Println("Supported login methods:")
		for _, m := range methods {
			fmt.Printf("* %v\n", m)
		}
		fmt.Println()
	} else {
		log.Fatalf("Can't list login methods: %v", err)
	}

	if err := session.BasicLogin("daniel", "danielpass"); err == nil {
		log.Printf("Successfully logged into Reva (token=%v)", session.Token())
		fmt.Println()
		runActions(session)
	} else {
		log.Fatalf("Can't log in to Reva: %v", err)
	}
}
