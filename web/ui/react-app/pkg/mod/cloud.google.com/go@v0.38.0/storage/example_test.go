// Copyright 2014 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage_test

import (
	"context"
	"fmt"
	"hash/crc32"
	"io"
	"io/ioutil"
	"log"
	"os"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

func ExampleNewClient() {
	ctx := context.Background()
	// Use Google Application Default Credentials to authorize and authenticate the client.
	// More information about Application Default Credentials and how to enable is at
	// https://developers.google.com/identity/protocols/application-default-credentials.
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	// Use the client.

	// Close the client when finished.
	if err := client.Close(); err != nil {
		// TODO: handle error.
	}
}

// This example shows how to create an unauthenticated client, which
// can be used to access public data.
func ExampleNewClient_unauthenticated() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx, option.WithoutAuthentication())
	if err != nil {
		// TODO: handle error.
	}
	// Use the client.

	// Close the client when finished.
	if err := client.Close(); err != nil {
		// TODO: handle error.
	}
}

func ExampleBucketHandle_Create() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	if err := client.Bucket("my-bucket").Create(ctx, "my-project", nil); err != nil {
		// TODO: handle error.
	}
}

func ExampleBucketHandle_Delete() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	if err := client.Bucket("my-bucket").Delete(ctx); err != nil {
		// TODO: handle error.
	}
}

func ExampleBucketHandle_Attrs() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	attrs, err := client.Bucket("my-bucket").Attrs(ctx)
	if err != nil {
		// TODO: handle error.
	}
	fmt.Println(attrs)
}

func ExampleBucketHandle_Update() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	// Enable versioning in the bucket, regardless of its previous value.
	attrs, err := client.Bucket("my-bucket").Update(ctx,
		storage.BucketAttrsToUpdate{VersioningEnabled: true})
	if err != nil {
		// TODO: handle error.
	}
	fmt.Println(attrs)
}

// If your update is based on the bucket's previous attributes, match the
// metageneration number to make sure the bucket hasn't changed since you read it.
func ExampleBucketHandle_Update_readModifyWrite() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	b := client.Bucket("my-bucket")
	attrs, err := b.Attrs(ctx)
	if err != nil {
		// TODO: handle error.
	}
	var au storage.BucketAttrsToUpdate
	au.SetLabel("lab", attrs.Labels["lab"]+"-more")
	if attrs.Labels["delete-me"] == "yes" {
		au.DeleteLabel("delete-me")
	}
	attrs, err = b.
		If(storage.BucketConditions{MetagenerationMatch: attrs.MetaGeneration}).
		Update(ctx, au)
	if err != nil {
		// TODO: handle error.
	}
	fmt.Println(attrs)
}

func ExampleClient_Buckets() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	it := client.Buckets(ctx, "my-bucket")
	_ = it // TODO: iterate using Next or iterator.Pager.
}

func ExampleBucketIterator_Next() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	it := client.Buckets(ctx, "my-project")
	for {
		bucketAttrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}
		fmt.Println(bucketAttrs)
	}
}

func ExampleBucketHandle_Objects() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	it := client.Bucket("my-bucket").Objects(ctx, nil)
	_ = it // TODO: iterate using Next or iterator.Pager.
}

func ExampleBucketHandle_AddNotification() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	b := client.Bucket("my-bucket")
	n, err := b.AddNotification(ctx, &storage.Notification{
		TopicProjectID: "my-project",
		TopicID:        "my-topic",
		PayloadFormat:  storage.JSONPayload,
	})
	if err != nil {
		// TODO: handle error.
	}
	fmt.Println(n.ID)
}

func ExampleBucketHandle_LockRetentionPolicy() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	b := client.Bucket("my-bucket")
	attrs, err := b.Attrs(ctx)
	if err != nil {
		// TODO: handle error.
	}
	// Note that locking the bucket without first attaching a RetentionPolicy
	// that's at least 1 day is a no-op
	err = b.If(storage.BucketConditions{MetagenerationMatch: attrs.MetaGeneration}).LockRetentionPolicy(ctx)
	if err != nil {
		// TODO: handle err
	}
}

func ExampleBucketHandle_Notifications() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	b := client.Bucket("my-bucket")
	ns, err := b.Notifications(ctx)
	if err != nil {
		// TODO: handle error.
	}
	for id, n := range ns {
		fmt.Printf("%s: %+v\n", id, n)
	}
}

var notificationID string

func ExampleBucketHandle_DeleteNotification() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	b := client.Bucket("my-bucket")
	// TODO: Obtain notificationID from BucketHandle.AddNotification
	// or BucketHandle.Notifications.
	err = b.DeleteNotification(ctx, notificationID)
	if err != nil {
		// TODO: handle error.
	}
}

func ExampleObjectIterator_Next() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	it := client.Bucket("my-bucket").Objects(ctx, nil)
	for {
		objAttrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}
		fmt.Println(objAttrs)
	}
}

func ExampleSignedURL() {
	pkey, err := ioutil.ReadFile("my-private-key.pem")
	if err != nil {
		// TODO: handle error.
	}
	url, err := storage.SignedURL("my-bucket", "my-object", &storage.SignedURLOptions{
		GoogleAccessID: "xxx@developer.gserviceaccount.com",
		PrivateKey:     pkey,
		Method:         "GET",
		Expires:        time.Now().Add(48 * time.Hour),
	})
	if err != nil {
		// TODO: handle error.
	}
	fmt.Println(url)
}

func ExampleObjectHandle_Attrs() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	objAttrs, err := client.Bucket("my-bucket").Object("my-object").Attrs(ctx)
	if err != nil {
		// TODO: handle error.
	}
	fmt.Println(objAttrs)
}

func ExampleObjectHandle_Attrs_withConditions() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	obj := client.Bucket("my-bucket").Object("my-object")
	// Read the object.
	objAttrs1, err := obj.Attrs(ctx)
	if err != nil {
		// TODO: handle error.
	}
	// Do something else for a while.
	time.Sleep(5 * time.Minute)
	// Now read the same contents, even if the object has been written since the last read.
	objAttrs2, err := obj.Generation(objAttrs1.Generation).Attrs(ctx)
	if err != nil {
		// TODO: handle error.
	}
	fmt.Println(objAttrs1, objAttrs2)
}

func ExampleObjectHandle_Update() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	// Change only the content type of the object.
	objAttrs, err := client.Bucket("my-bucket").Object("my-object").Update(ctx, storage.ObjectAttrsToUpdate{
		ContentType:        "text/html",
		ContentDisposition: "", // delete ContentDisposition
	})
	if err != nil {
		// TODO: handle error.
	}
	fmt.Println(objAttrs)
}

func ExampleObjectHandle_NewReader() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	rc, err := client.Bucket("my-bucket").Object("my-object").NewReader(ctx)
	if err != nil {
		// TODO: handle error.
	}
	slurp, err := ioutil.ReadAll(rc)
	rc.Close()
	if err != nil {
		// TODO: handle error.
	}
	fmt.Println("file contents:", slurp)
}

func ExampleObjectHandle_NewRangeReader() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	// Read only the first 64K.
	rc, err := client.Bucket("bucketname").Object("filename1").NewRangeReader(ctx, 0, 64*1024)
	if err != nil {
		// TODO: handle error.
	}
	slurp, err := ioutil.ReadAll(rc)
	rc.Close()
	if err != nil {
		// TODO: handle error.
	}
	fmt.Println("first 64K of file contents:", slurp)
}

func ExampleObjectHandle_NewWriter() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	wc := client.Bucket("bucketname").Object("filename1").NewWriter(ctx)
	_ = wc // TODO: Use the Writer.
}

func ExampleWriter_Write() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	wc := client.Bucket("bucketname").Object("filename1").NewWriter(ctx)
	wc.ContentType = "text/plain"
	wc.ACL = []storage.ACLRule{{Entity: storage.AllUsers, Role: storage.RoleReader}}
	if _, err := wc.Write([]byte("hello world")); err != nil {
		// TODO: handle error.
		// Note that Write may return nil in some error situations,
		// so always check the error from Close.
	}
	if err := wc.Close(); err != nil {
		// TODO: handle error.
	}
	fmt.Println("updated object:", wc.Attrs())
}

// To limit the time to write an object (or do anything else
// that takes a context), use context.WithTimeout.
func ExampleWriter_Write_timeout() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel() // Cancel when done, whether we time out or not.
	wc := client.Bucket("bucketname").Object("filename1").NewWriter(tctx)
	wc.ContentType = "text/plain"
	wc.ACL = []storage.ACLRule{{Entity: storage.AllUsers, Role: storage.RoleReader}}
	if _, err := wc.Write([]byte("hello world")); err != nil {
		// TODO: handle error.
		// Note that Write may return nil in some error situations,
		// so always check the error from Close.
	}
	if err := wc.Close(); err != nil {
		// TODO: handle error.
	}
	fmt.Println("updated object:", wc.Attrs())
}

// To make sure the data you write is uncorrupted, use an MD5 or CRC32c
// checksum. This example illustrates CRC32c.
func ExampleWriter_Write_checksum() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	data := []byte("verify me")
	wc := client.Bucket("bucketname").Object("filename1").NewWriter(ctx)
	wc.CRC32C = crc32.Checksum(data, crc32.MakeTable(crc32.Castagnoli))
	wc.SendCRC32C = true
	if _, err := wc.Write([]byte("hello world")); err != nil {
		// TODO: handle error.
		// Note that Write may return nil in some error situations,
		// so always check the error from Close.
	}
	if err := wc.Close(); err != nil {
		// TODO: handle error.
	}
	fmt.Println("updated object:", wc.Attrs())
}

func ExampleObjectHandle_Delete() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	// To delete multiple objects in a bucket, list them with an
	// ObjectIterator, then Delete them.

	// If you are using this package on the App Engine Flex runtime,
	// you can init a bucket client with your app's default bucket name.
	// See http://godoc.org/google.golang.org/appengine/file#DefaultBucketName.
	bucket := client.Bucket("my-bucket")
	it := bucket.Objects(ctx, nil)
	for {
		objAttrs, err := it.Next()
		if err != nil && err != iterator.Done {
			// TODO: Handle error.
		}
		if err == iterator.Done {
			break
		}
		if err := bucket.Object(objAttrs.Name).Delete(ctx); err != nil {
			// TODO: Handle error.
		}
	}
	fmt.Println("deleted all object items in the bucket specified.")
}

func ExampleACLHandle_Delete() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	// No longer grant access to the bucket to everyone on the Internet.
	if err := client.Bucket("my-bucket").ACL().Delete(ctx, storage.AllUsers); err != nil {
		// TODO: handle error.
	}
}

func ExampleACLHandle_Set() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	// Let any authenticated user read my-bucket/my-object.
	obj := client.Bucket("my-bucket").Object("my-object")
	if err := obj.ACL().Set(ctx, storage.AllAuthenticatedUsers, storage.RoleReader); err != nil {
		// TODO: handle error.
	}
}

func ExampleACLHandle_List() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	// List the default object ACLs for my-bucket.
	aclRules, err := client.Bucket("my-bucket").DefaultObjectACL().List(ctx)
	if err != nil {
		// TODO: handle error.
	}
	fmt.Println(aclRules)
}

func ExampleCopier_Run() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	src := client.Bucket("bucketname").Object("file1")
	dst := client.Bucket("another-bucketname").Object("file2")

	// Copy content and modify metadata.
	copier := dst.CopierFrom(src)
	copier.ContentType = "text/plain"
	attrs, err := copier.Run(ctx)
	if err != nil {
		// TODO: Handle error, possibly resuming with copier.RewriteToken.
	}
	fmt.Println(attrs)

	// Just copy content.
	attrs, err = dst.CopierFrom(src).Run(ctx)
	if err != nil {
		// TODO: Handle error. No way to resume.
	}
	fmt.Println(attrs)
}

func ExampleCopier_Run_progress() {
	// Display progress across multiple rewrite RPCs.
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	src := client.Bucket("bucketname").Object("file1")
	dst := client.Bucket("another-bucketname").Object("file2")

	copier := dst.CopierFrom(src)
	copier.ProgressFunc = func(copiedBytes, totalBytes uint64) {
		log.Printf("copy %.1f%% done", float64(copiedBytes)/float64(totalBytes)*100)
	}
	if _, err := copier.Run(ctx); err != nil {
		// TODO: handle error.
	}
}

var key1, key2 []byte

func ExampleObjectHandle_CopierFrom_rotateEncryptionKeys() {
	// To rotate the encryption key on an object, copy it onto itself.
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	obj := client.Bucket("bucketname").Object("obj")
	// Assume obj is encrypted with key1, and we want to change to key2.
	_, err = obj.Key(key2).CopierFrom(obj.Key(key1)).Run(ctx)
	if err != nil {
		// TODO: handle error.
	}
}

func ExampleComposer_Run() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	bkt := client.Bucket("bucketname")
	src1 := bkt.Object("o1")
	src2 := bkt.Object("o2")
	dst := bkt.Object("o3")
	// Compose and modify metadata.
	c := dst.ComposerFrom(src1, src2)
	c.ContentType = "text/plain"
	attrs, err := c.Run(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(attrs)
	// Just compose.
	attrs, err = dst.ComposerFrom(src1, src2).Run(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(attrs)
}

var gen int64

func ExampleObjectHandle_Generation() {
	// Read an object's contents from generation gen, regardless of the
	// current generation of the object.
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	obj := client.Bucket("my-bucket").Object("my-object")
	rc, err := obj.Generation(gen).NewReader(ctx)
	if err != nil {
		// TODO: handle error.
	}
	defer rc.Close()
	if _, err := io.Copy(os.Stdout, rc); err != nil {
		// TODO: handle error.
	}
}

func ExampleObjectHandle_If() {
	// Read from an object only if the current generation is gen.
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	obj := client.Bucket("my-bucket").Object("my-object")
	rc, err := obj.If(storage.Conditions{GenerationMatch: gen}).NewReader(ctx)
	if err != nil {
		// TODO: handle error.
	}
	defer rc.Close()
	if _, err := io.Copy(os.Stdout, rc); err != nil {
		// TODO: handle error.
	}
}

var secretKey []byte

func ExampleObjectHandle_Key() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	obj := client.Bucket("my-bucket").Object("my-object")
	// Encrypt the object's contents.
	w := obj.Key(secretKey).NewWriter(ctx)
	if _, err := w.Write([]byte("top secret")); err != nil {
		// TODO: handle error.
	}
	if err := w.Close(); err != nil {
		// TODO: handle error.
	}
}
