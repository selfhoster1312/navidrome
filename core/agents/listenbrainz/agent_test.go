package listenbrainz

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core/scrobbler"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("listenBrainzAgent", func() {
	var ds model.DataStore
	var ctx context.Context
	var agent *listenBrainzAgent
	var httpClient *tests.FakeHttpClient
	var track *model.MediaFile

	BeforeEach(func() {
		ds = &tests.MockDataStore{}
		ctx = context.Background()
		_ = ds.UserProps(ctx).Put("user-1", sessionKeyProperty, "SK-1")
		httpClient = &tests.FakeHttpClient{}
		agent = listenBrainzConstructor(ds)
		agent.client = NewClient("http://localhost:8080", httpClient)
		track = &model.MediaFile{
			ID:          "123",
			Title:       "Track Title",
			Album:       "Track Album",
			Artist:      "Track Artist",
			TrackNumber: 1,
			MbzTrackID:  "mbz-123",
			MbzAlbumID:  "mbz-456",
			MbzArtistID: "mbz-789",
		}
	})

	Describe("formatListen", func() {
		It("constructs the listenInfo properly", func() {
			var idArtistId = func(element interface{}) string {
				return element.(string)
			}

			lr := agent.formatListen(track)
			Expect(lr).To(MatchAllFields(Fields{
				"ListenedAt": Equal(0),
				"TrackMetadata": MatchAllFields(Fields{
					"ArtistName":  Equal(track.Artist),
					"TrackName":   Equal(track.Title),
					"ReleaseName": Equal(track.Album),
					"AdditionalInfo": MatchAllFields(Fields{
						"SubmissionClient":        Equal(consts.AppName),
						"SubmissionClientVersion": Equal(consts.Version),
						"TrackNumber":             Equal(track.TrackNumber),
						"TrackMbzID":              Equal(track.MbzTrackID),
						"ReleaseMbID":             Equal(track.MbzAlbumID),
						"ArtistMbzIDs": MatchAllElements(idArtistId, Elements{
							"mbz-789": Equal(track.MbzArtistID),
						}),
					}),
				}),
			}))
		})
	})

	Describe("NowPlaying", func() {
		It("updates NowPlaying successfully", func() {
			httpClient.Res = http.Response{Body: io.NopCloser(bytes.NewBufferString(`{"status": "ok"}`)), StatusCode: 200}

			err := agent.NowPlaying(ctx, "user-1", track)
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns ErrNotAuthorized if user is not linked", func() {
			err := agent.NowPlaying(ctx, "user-2", track)
			Expect(err).To(MatchError(scrobbler.ErrNotAuthorized))
		})
	})

	Describe("Scrobble", func() {
		var sc scrobbler.Scrobble

		BeforeEach(func() {
			sc = scrobbler.Scrobble{MediaFile: *track, TimeStamp: time.Now()}
		})

		It("sends a Scrobble successfully", func() {
			httpClient.Res = http.Response{Body: io.NopCloser(bytes.NewBufferString(`{"status": "ok"}`)), StatusCode: 200}

			err := agent.Scrobble(ctx, "user-1", sc)
			Expect(err).ToNot(HaveOccurred())
		})

		It("sets the Timestamp properly", func() {
			httpClient.Res = http.Response{Body: io.NopCloser(bytes.NewBufferString(`{"status": "ok"}`)), StatusCode: 200}

			err := agent.Scrobble(ctx, "user-1", sc)
			Expect(err).ToNot(HaveOccurred())

			decoder := json.NewDecoder(httpClient.SavedRequest.Body)
			var lr listenBrainzRequestBody
			err = decoder.Decode(&lr)

			Expect(err).ToNot(HaveOccurred())
			Expect(lr.Payload[0].ListenedAt).To(Equal(int(sc.TimeStamp.Unix())))
		})

		It("returns ErrNotAuthorized if user is not linked", func() {
			err := agent.Scrobble(ctx, "user-2", sc)
			Expect(err).To(MatchError(scrobbler.ErrNotAuthorized))
		})

		It("returns ErrRetryLater on error 503", func() {
			httpClient.Res = http.Response{
				Body:       io.NopCloser(bytes.NewBufferString(`{"code": 503, "error": "Cannot submit listens to queue, please try again later."}`)),
				StatusCode: 503,
			}

			err := agent.Scrobble(ctx, "user-1", sc)
			Expect(err).To(MatchError(scrobbler.ErrRetryLater))
		})

		It("returns ErrRetryLater on error 500", func() {
			httpClient.Res = http.Response{
				Body:       io.NopCloser(bytes.NewBufferString(`{"code": 500, "error": "Something went wrong. Please try again."}`)),
				StatusCode: 500,
			}

			err := agent.Scrobble(ctx, "user-1", sc)
			Expect(err).To(MatchError(scrobbler.ErrRetryLater))
		})

		It("returns ErrRetryLater on http errors", func() {
			httpClient.Res = http.Response{
				Body:       io.NopCloser(bytes.NewBufferString(`Bad Gateway`)),
				StatusCode: 500,
			}

			err := agent.Scrobble(ctx, "user-1", sc)
			Expect(err).To(MatchError(scrobbler.ErrRetryLater))
		})

		It("returns ErrUnrecoverable on other errors", func() {
			httpClient.Res = http.Response{
				Body:       io.NopCloser(bytes.NewBufferString(`{"code": 400, "error": "BadRequest: Invalid JSON document submitted."}`)),
				StatusCode: 400,
			}

			err := agent.Scrobble(ctx, "user-1", sc)
			Expect(err).To(MatchError(scrobbler.ErrUnrecoverable))
		})
	})
})
