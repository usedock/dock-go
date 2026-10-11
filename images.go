package dock

import "context"

// Image is a Dockyard image a dock can be created from.
type Image struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Arch        []string `json:"arch,omitempty"`
	SizeMiB     int      `json:"sizeMiB,omitempty"`
	Contents    string   `json:"contents,omitempty"`
	// Available reports whether a host can serve the image right now.
	Available bool `json:"available"`
}

// ImagesService is the Dockyard image catalog. Get it from Client.Images.
type ImagesService struct{ c *Client }

// List returns the Dockyard images. Pass a Name as CreateParams.Image.
func (s *ImagesService) List(ctx context.Context) ([]Image, error) {
	var out []Image
	if err := s.c.do(ctx, "GET", "/images", nil, nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
