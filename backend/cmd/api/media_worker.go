package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/minio/minio-go/v7"
)

const (
	losslessWebPSuffix  = ".lossless.webp"
	optimizedWebPSuffix = ".webp"
	mediaJobMaxAttempts = 5
)

type mediaConversionJob struct {
	mediaID   string
	spaceID   string
	sourceKey *string
}

func (s *server) runMediaWorker(ctx context.Context) {
	if s.db == nil || s.minio == nil {
		return
	}
	_, _ = s.db.Exec(ctx, "update media_conversion_jobs set status='pending',locked_at=null where status='processing'")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.runNextMediaConversion(ctx); err != nil {
			time.Sleep(time.Second)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *server) runNextMediaConversion(ctx context.Context) error {
	job, err := s.claimMediaConversion(ctx)
	if err != nil || job == nil {
		return err
	}
	if err := s.convertMediaJob(ctx, *job); err != nil {
		s.failMediaConversion(ctx, job.mediaID, err)
		return err
	}
	return nil
}

func (s *server) claimMediaConversion(ctx context.Context) (*mediaConversionJob, error) {
	const query = `
		with next as (
			select j.media_id from media_conversion_jobs j
			join media m on m.id=j.media_id and m.deleted_at is null
			where j.status='pending' and j.next_attempt_at <= now()
			order by j.created_at
			for update skip locked
			limit 1
		)
		update media_conversion_jobs j
		set status='processing', attempts=attempts+1, locked_at=now(), last_error=null
		from next join media m on m.id=next.media_id
		where j.media_id=next.media_id
		returning j.media_id, m.storage_space_id, j.source_object_key`
	job := mediaConversionJob{}
	if err := s.db.QueryRow(ctx, query).Scan(&job.mediaID, &job.spaceID, &job.sourceKey); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &job, nil
}

func (s *server) convertMediaJob(ctx context.Context, job mediaConversionJob) error {
	sourceKey := ""
	if job.sourceKey != nil {
		sourceKey = *job.sourceKey
	}
	if sourceKey == "" {
		var err error
		sourceKey, err = s.findMediaSourceObject(ctx, job.spaceID, job.mediaID)
		if err != nil {
			return err
		}
	}
	source, err := s.minio.GetObject(ctx, s.mediaBucket, sourceKey, minio.GetObjectOptions{})
	if err != nil {
		return err
	}
	defer source.Close()
	data, err := io.ReadAll(source)
	if err != nil {
		return err
	}
	image, err := decodeUploadImage(data)
	if err != nil {
		return err
	}
	lossless, err := encodeWebP(image, true, 100)
	if err != nil {
		return err
	}
	optimized, err := encodeWebP(image, false, 75)
	if err != nil {
		return err
	}
	losslessKey, optimizedKey := mediaVariantKeys(job.spaceID, job.mediaID)
	if _, err = s.minio.PutObject(ctx, s.mediaBucket, losslessKey, bytes.NewReader(lossless), int64(len(lossless)), minio.PutObjectOptions{ContentType: "image/webp"}); err != nil {
		return err
	}
	if _, err = s.minio.PutObject(ctx, s.mediaBucket, optimizedKey, bytes.NewReader(optimized), int64(len(optimized)), minio.PutObjectOptions{ContentType: "image/webp"}); err != nil {
		_ = s.minio.RemoveObject(ctx, s.mediaBucket, losslessKey, minio.RemoveObjectOptions{})
		return err
	}
	if _, err = s.db.Exec(ctx, "update media set object_key=$1,mime_type='image/webp',byte_size=$2 where id=$3", optimizedKey, len(optimized), job.mediaID); err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, "update media_conversion_jobs set status='completed',completed_at=now(),locked_at=null,last_error=null,source_object_key=$1 where media_id=$2", sourceKey, job.mediaID)
	return err
}

func (s *server) failMediaConversion(ctx context.Context, mediaID string, cause error) {
	_, _ = s.db.Exec(ctx, `
		update media_conversion_jobs
		set status=case when attempts >= $1 then 'failed' else 'pending' end,
			next_attempt_at=now() + interval '30 seconds', locked_at=null, last_error=$2
		where media_id=$3`, mediaJobMaxAttempts, cause.Error(), mediaID)
}

func (s *server) findMediaSourceObject(ctx context.Context, spaceID, mediaID string) (string, error) {
	base := "spaces/" + spaceID + "/media/" + mediaID
	losslessKey, optimizedKey := mediaVariantKeys(spaceID, mediaID)
	fallback := ""
	for object := range s.minio.ListObjects(ctx, s.mediaBucket, minio.ListObjectsOptions{Prefix: base + ".", Recursive: true}) {
		if object.Err != nil {
			return "", object.Err
		}
		if object.Key == losslessKey || object.Key == optimizedKey {
			continue
		}
		if strings.HasSuffix(object.Key, ".original.webp") || !strings.HasSuffix(object.Key, ".webp") {
			return object.Key, nil
		}
		fallback = object.Key
	}
	if fallback != "" {
		return fallback, nil
	}
	return "", fmt.Errorf("source object not found for media %s", mediaID)
}

func mediaVariantKeys(spaceID, mediaID string) (string, string) {
	base := "spaces/" + spaceID + "/media/" + mediaID
	return base + losslessWebPSuffix, base + optimizedWebPSuffix
}
