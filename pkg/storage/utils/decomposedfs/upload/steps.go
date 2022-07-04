package upload

import (
	"os"
	"time"

	"github.com/cs3org/reva/v2/pkg/antivirus"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/options"
	"github.com/cs3org/reva/v2/pkg/utils/postprocessing"
	"github.com/pkg/errors"
)

// Initialize is the step that initializes the node
func Initialize(upload *Upload) postprocessing.Step {
	return postprocessing.NewStep("initialize", func() error {
		// we need the node to start processing
		n, err := CreateNodeForUpload(upload)
		if err != nil {
			return err
		}

		// set processing status
		upload.node = n
		return upload.node.MarkProcessing()
	}, nil)
}

// Assemble assembles the file and moves it to the blobstore
func Assemble(upload *Upload, async bool, waitforscan bool) postprocessing.Step {
	requires := []string{"initialize"}
	if waitforscan {
		requires = append(requires, "scanning")
	}
	return postprocessing.NewStep("assembling", func() error {
		err := upload.finishUpload()
		if !async && upload.node != nil {
			_ = upload.node.UnmarkProcessing() // NOTE: this makes the testsuite happy - remove once adjusted
		}
		return err
	}, upload.cleanup, requires...)
}

// Scan scans the file for viruses
func Scan(upload *Upload, avType string, handle string) postprocessing.Step {
	return postprocessing.NewStep("scanning", func() error {
		f, err := os.Open(upload.binPath)
		if err != nil {
			return err
		}

		scanner, err := antivirus.New(avType)
		if err != nil {
			return err
		}

		result, err := scanner.Scan(f)
		if err != nil {
			// What to do when there was an error while scanning? -> file should stay in uploadpath for now
			return err
		}

		if !result.Infected {
			// all good
			return nil
		}

		// TODO: send email that file was infected

		switch options.InfectedFileOption(handle) {
		default:
			fallthrough
		case options.Delete:
			upload.cleanup(errors.New("infected"))
			upload.node = nil
			return nil
		case options.Error:
			return errors.New("file infected")
		case options.Ignore:
			return nil
		}

		return nil
	}, nil, "initialize")
}

// Sleep just waits for the given time
func Sleep(_ *Upload, sleeptime time.Duration) postprocessing.Step {
	return postprocessing.NewStep("sleep", func() error {
		time.Sleep(sleeptime)
		return nil
	}, nil)
}
