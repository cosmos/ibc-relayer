package ibcv2_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/cosmos/ibc-relayer/shared/bridges/ibcv2"
)

func TestOnceWithKey_Do(t *testing.T) {
	t.Run("single key, action returns without error, value is always cached", func(t *testing.T) {
		key := "key"
		ret := "0xhash"
		called := 0
		action := func() (*string, error) {
			time.Sleep(1 * time.Second)
			called++
			return &ret, nil
		}

		once := ibcv2.NewOnceWithKey[*string](time.Minute, 500*time.Millisecond)

		var wg sync.WaitGroup
		for range 5 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				value, err := once.Do(key, action)
				assert.NoError(t, err)
				assert.Equal(t, &ret, value)
			}()
		}

		wg.Wait()
		assert.Equal(t, 1, called)
	})

	t.Run("single key, action is timed out on successful invocation after success timeout passes", func(t *testing.T) {
		key := "key"
		ret := "0xhash"
		called := 0
		action := func() (*string, error) {
			called++
			return &ret, nil
		}

		once := ibcv2.NewOnceWithKey[*string](time.Second*3, 500*time.Millisecond)

		// should only call once
		var wg sync.WaitGroup
		for range 5 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				value, err := once.Do(key, action)
				assert.NoError(t, err)
				assert.Equal(t, &ret, value)
			}()
		}

		wg.Wait()
		assert.Equal(t, 1, called)

		time.Sleep(time.Second * 3)

		// should call again since the success timed out
		for range 5 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				value, err := once.Do(key, action)
				assert.NoError(t, err)
				assert.Equal(t, &ret, value)
			}()
		}

		wg.Wait()
		assert.Equal(t, 2, called)
	})

	t.Run("single key, action is success timed out while another call is executing it", func(t *testing.T) {
		key := "key"
		ret := "0xhash"

		// need this because the tests try to inc called at the same time
		lock := sync.Mutex{}
		called := 0
		action := func() (*string, error) {
			time.Sleep(5 * time.Second)
			lock.Lock()
			defer lock.Unlock()
			called++
			return &ret, nil
		}

		once := ibcv2.NewOnceWithKey[*string](1*time.Second, 500*time.Millisecond)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := once.Do(key, action)
			assert.NoError(t, err)
			assert.Equal(t, &ret, value)
		}()

		time.Sleep(1 * time.Second)
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := once.Do(key, action)
			assert.NoError(t, err)
			assert.Equal(t, &ret, value)
		}()

		wg.Wait()

		assert.Equal(t, 2, called)
	})

	t.Run("multiple keys, action returns without error, value is always cached", func(t *testing.T) {
		key1 := "key1"
		key1ret := "0xhash1"
		key1called := 0
		action1 := func() (*string, error) {
			time.Sleep(1 * time.Second)
			key1called++
			return &key1ret, nil
		}

		key2 := "key2"
		key2ret := "0xhash2"
		key2called := 0
		action2 := func() (*string, error) {
			time.Sleep(1 * time.Second)
			key2called++
			return &key2ret, nil
		}

		once := ibcv2.NewOnceWithKey[*string](time.Minute, 500*time.Millisecond)

		var wg sync.WaitGroup
		for range 5 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				value, err := once.Do(key1, action1)
				assert.NoError(t, err)
				assert.Equal(t, &key1ret, value)
			}()

			wg.Add(1)
			go func() {
				defer wg.Done()
				value, err := once.Do(key2, action2)
				assert.NoError(t, err)
				assert.Equal(t, &key2ret, value)
			}()
		}

		wg.Wait()
		assert.Equal(t, 1, key1called)
		assert.Equal(t, 1, key2called)
	})

	t.Run("single key, errors and times out, then returns value", func(t *testing.T) {
		key := "key"
		ret := "0xhash"
		targetErr := fmt.Errorf("err")
		called := 0
		shouldReturnProperly := time.Now().Add(5 * time.Second)
		action := func() (*string, error) {
			called++
			if time.Now().After(shouldReturnProperly) {
				return &ret, nil
			}
			return nil, targetErr
		}

		once := ibcv2.NewOnceWithKey[*string](time.Minute, 1*time.Second)

		// call the action 5 times, expect for the value to not have been timed
		// out by now so only expect for it to have been called once
		var wg sync.WaitGroup
		for range 5 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				value, err := once.Do(key, action)
				assert.ErrorIs(t, err, targetErr)
				assert.Nil(t, value)
			}()
		}
		wg.Wait()
		assert.Equal(t, 1, called)

		time.Sleep(1 * time.Second)
		// after sleeping, the original call that errored should be timed out,
		// and we will now retry the action once, however we will still receive the error

		// call the action 5 more times, expect for the value to not have been timed
		// out by now so only expect for it to have been called once
		for range 5 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				value, err := once.Do(key, action)
				assert.ErrorIs(t, err, targetErr)
				assert.Nil(t, value)
			}()
		}
		wg.Wait()
		assert.Equal(t, 2, called)

		time.Sleep(5 * time.Second)
		// sleep until the action will return the proper value, ensure that
		// this again was only called once, even after waiting for the error
		// timeout (since this call will not error)

		// call the action 5 more times, expect for the value to not have been timed
		// out by now so only expect for it to have been called once
		for range 5 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				value, err := once.Do(key, action)
				assert.NoError(t, err)
				assert.Equal(t, &ret, value)
			}()
		}
		wg.Wait()
		assert.Equal(t, 3, called)
	})
}
